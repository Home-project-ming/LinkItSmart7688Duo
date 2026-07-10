#!/bin/sh
# Controlled experiment: SAME binary, three storage locations.
# Isolates "where the executable lives" from "which binary it is".

BB=/rom/bin/busybox
[ -f "$BB" ] || BB=/bin/busybox

echo "=== control binary ==="
ls -l "$BB"
busybox md5sum "$BB"

# 1. flash / squashfs (original)
# 2. SD / ext4 (via the extroot overlay upperdir, direct path)
# 3. tmpfs (RAM)
mkdir -p /overlay/upper/exectest /tmp/exectest
cp "$BB" /overlay/upper/exectest/bb_sd
cp "$BB" /tmp/exectest/bb_ram
chmod 755 /overlay/upper/exectest/bb_sd /tmp/exectest/bb_ram
sync

echo
echo "=== verify all three are byte-identical ==="
busybox md5sum "$BB" /overlay/upper/exectest/bb_sd /tmp/exectest/bb_ram

probe() {   # probe <label> <path>
    label="$1"; path="$2"
    ok=0; ill=0; seg=0; other=0
    i=1
    while [ $i -le 10 ]; do
        sync
        echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
        "$path" true >/dev/null 2>&1
        rc=$?
        case $rc in
            0)   ok=$((ok+1)) ;;
            132) ill=$((ill+1)) ;;
            139) seg=$((seg+1)) ;;
            *)   other=$((other+1)) ;;
        esac
        i=$((i + 1))
    done
    printf '  %-42s ok=%-3s SIGILL=%-3s SIGSEGV=%-3s other=%s\n' "$label" "$ok" "$ill" "$seg" "$other"
}

echo
echo "=== run 'busybox true' 10x from each location, cache dropped every time ==="
probe "flash  (squashfs, /rom)"  "$BB"
probe "SD     (ext4, /overlay/upper)" /overlay/upper/exectest/bb_sd
probe "RAM    (tmpfs, /tmp)"     /tmp/exectest/bb_ram

echo
echo "=== which MMC driver actually bound? ==="
dmesg | grep -i -e 'mtk-sd' -e sdhci -e 'mmc0' | head -6
echo "--- driver in sysfs ---"
ls -l /sys/class/mmc_host/mmc0/device/driver 2>/dev/null | sed 's/.*-> //'
readlink /sys/block/mmcblk0/device/../driver 2>/dev/null

echo
echo "=== DMA in use? ==="
grep -i -e mmc -e sd /proc/interrupts | head -4

echo
echo "=== cleanup ==="
rm -rf /overlay/upper/exectest /tmp/exectest
sync
echo done
