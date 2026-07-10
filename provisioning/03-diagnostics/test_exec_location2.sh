#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Same binary, three storage locations. Keep the filename 'busybox' so the
# multi-call dispatch on argv[0] still works (previous run's rc=127 bug).

BB=/rom/bin/busybox
[ -f "$BB" ] || BB=/bin/busybox

rm -rf /overlay/upper/exectest /tmp/exectest
mkdir -p /overlay/upper/exectest /tmp/exectest
cp "$BB" /overlay/upper/exectest/busybox
cp "$BB" /tmp/exectest/busybox
chmod 755 /overlay/upper/exectest/busybox /tmp/exectest/busybox
sync

echo "=== all three byte-identical? ==="
busybox md5sum "$BB" /overlay/upper/exectest/busybox /tmp/exectest/busybox

echo
echo "=== smoke test each once (no cache drop) ==="
"$BB" true && echo "  flash ok"
/overlay/upper/exectest/busybox true && echo "  SD ok"
/tmp/exectest/busybox true && echo "  RAM ok"

probe() {
    label="$1"; path="$2"
    ok=0; ill=0; seg=0; other=0; codes=""
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
            *)   other=$((other+1)); codes="$codes $rc" ;;
        esac
        i=$((i + 1))
    done
    printf '  %-40s ok=%-3s SIGILL=%-3s SIGSEGV=%-3s other=%s%s\n' \
        "$label" "$ok" "$ill" "$seg" "$other" "$codes"
}

echo
echo "=== 'busybox true' x10 from each location, drop_caches each time ==="
probe "flash (squashfs /rom)"          "$BB"
probe "SD    (ext4 /overlay/upper)"    /overlay/upper/exectest/busybox
probe "RAM   (tmpfs /tmp)"             /tmp/exectest/busybox

echo
echo "=== and the real Entware binary, for comparison ==="
probe "SD    (/opt/bin/python3)"       /opt/bin/python3

echo
echo "=== cleanup ==="
rm -rf /overlay/upper/exectest /tmp/exectest
sync
echo done
