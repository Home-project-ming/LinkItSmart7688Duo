#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Refined hypothesis: the mmap/readahead page-fault path from ext4 on mtk-sd
# delivers corrupt pages, while read() does not. Small binaries rarely hit it;
# python3 (thousands of pages) hits it every time.
#
# Prediction: setting read_ahead_kb = 0 should reduce or eliminate the crashes.

RA=/sys/block/mmcblk0/queue/read_ahead_kb

probe() {
    label="$1"; shift
    ok=0; ill=0; seg=0; bus=0; other=0
    i=1
    while [ $i -le 10 ]; do
        sync
        echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
        "$@" >/dev/null 2>&1
        rc=$?
        case $rc in
            0)   ok=$((ok+1)) ;;
            132) ill=$((ill+1)) ;;
            138) bus=$((bus+1)) ;;
            139) seg=$((seg+1)) ;;
            *)   other=$((other+1)) ;;
        esac
        i=$((i + 1))
    done
    printf '  %-34s ok=%-3s ILL=%-3s BUS=%-3s SEGV=%-3s other=%s\n' \
        "$label" "$ok" "$ill" "$bus" "$seg" "$other"
}

echo "=== small Entware (glibc) binaries, cold cache ==="
probe "/opt/bin/grep --version"  /opt/bin/grep --version
probe "/opt/bin/find --version"  /opt/bin/find --version

echo
echo "=== current readahead ==="
cat $RA 2>/dev/null || echo "(no $RA)"

echo
echo "=== BASELINE: python3 with default readahead ==="
probe "python3 --version"  /opt/bin/python3 --version

echo
echo "=== set read_ahead_kb = 0 ==="
echo 0 > $RA 2>/dev/null && echo "  now: $(cat $RA)" || echo "  FAILED to set"

echo
echo "=== python3 with readahead disabled ==="
probe "python3 --version"  /opt/bin/python3 --version
probe "/opt/bin/grep --version"  /opt/bin/grep --version

echo
echo "=== restore readahead to 128 ==="
echo 128 > $RA 2>/dev/null && echo "  now: $(cat $RA)"

echo
echo "=== confirm crashes come back with readahead on ==="
probe "python3 --version"  /opt/bin/python3 --version

echo
echo "=== kernel log after all this ==="
dmesg | grep -i -e mmc -e ext4 -e error -e 'unaligned' | tail -6 || echo "  (nothing)"

echo
echo "=== memory ==="
free | sed -n '2p'
