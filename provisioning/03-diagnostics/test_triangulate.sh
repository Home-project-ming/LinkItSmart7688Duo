#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Triangulate: is the fault "files live on SD" or "Entware glibc"?
# tmpfs pages are NOT dropped by drop_caches, so RAM-resident copies stay warm.
#
#   a) loader SD  + libs SD  + exe SD    (status quo)
#   b) loader RAM + libs RAM + exe RAM   (nothing on SD)
#   c) loader RAM + libs SD  + exe SD    (only loader moved)

set -e
E=/tmp/e
rm -rf $E; mkdir -p $E

echo "=== size of /opt/lib top-level shared objects ==="
busybox du -sh /opt/lib 2>/dev/null
busybox ls -l /opt/lib/*.so* 2>/dev/null | busybox awk '{s+=$5} END {print "  top-level .so bytes:", s}'

echo
echo "=== copy loader + libs + grep into tmpfs ==="
cp /opt/lib/ld-2.27.so $E/ 2>/dev/null || cp /opt/lib/ld.so.1 $E/ld-2.27.so
for f in /opt/lib/*.so*; do cp -a "$f" $E/ 2>/dev/null || true; done
cp /opt/bin/grep $E/grep
chmod 755 $E/grep $E/ld-2.27.so
sync
busybox du -sh $E
busybox df -h /tmp | busybox tail -1
set +e

probe() {
    label="$1"; shift
    ok=0; ill=0; bus=0; seg=0; other=0
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
    printf '  %-46s ok=%-3s ILL=%-3s BUS=%-3s SEGV=%-3s other=%s\n' \
        "$label" "$ok" "$ill" "$bus" "$seg" "$other"
}

echo
echo "=== 10x each, cold cache ==="
probe "a) all on SD" \
    /opt/lib/ld.so.1 --library-path /opt/lib /opt/bin/grep --version
probe "b) all in RAM (tmpfs)" \
    $E/ld-2.27.so --library-path $E $E/grep --version
probe "c) loader RAM, libs+exe SD" \
    $E/ld-2.27.so --library-path /opt/lib /opt/bin/grep --version

echo
echo "=== control: busybox (uClibc, libs on flash), correct argv0 ==="
mkdir -p /tmp/bbdir
cp /rom/bin/busybox /tmp/bbdir/busybox 2>/dev/null || cp /bin/busybox /tmp/bbdir/busybox
chmod 755 /tmp/bbdir/busybox
probe "d) busybox from tmpfs" /tmp/bbdir/busybox true
probe "e) busybox from flash" /rom/bin/busybox true

echo
echo "=== cleanup ==="
rm -rf $E /tmp/bbdir
sync
echo done
