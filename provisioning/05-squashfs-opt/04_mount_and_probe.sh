#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# THE experiment: loop-mount /overlay/opt.sqfs and run the exact
# cold-cache probe from 03-diagnostics/test_triangulate.sh against the
# same binaries in both locations:
#   a) control: loader+libs+exe from /opt (ext4 on SD) - historical 0/10
#   b) experiment: identical files from the squashfs image ON THE SAME
#      SD CARD, loop-mounted (pages are CPU-decompressed, not DMA-filled)
#   c) sanity: busybox from flash squashfs - historical 10/10
#   d) bonus: python3 --version from both locations
M=/mnt/opt-sq
mkdir -p $M
umount $M 2>/dev/null
losetup -d /dev/loop0 2>/dev/null
losetup /dev/loop0 /overlay/opt.sqfs
mount -t squashfs -o ro /dev/loop0 $M || { echo "MOUNT FAILED"; exit 1; }
echo "--- mounted:"
mount | grep "$M"
busybox df -h $M | busybox tail -1

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
    printf '  %-44s ok=%-3s ILL=%-3s BUS=%-3s SEGV=%-3s other=%s\n' \
        "$label" "$ok" "$ill" "$bus" "$seg" "$other"
}

echo
echo "=== 10x each, cold cache ==="
probe "a) grep, ext4 SD (control)" \
    /opt/lib/ld.so.1 --library-path /opt/lib /opt/bin/grep --version
probe "b) grep, squashfs loop on same SD" \
    $M/lib/ld.so.1 --library-path $M/lib $M/bin/grep --version
probe "c) busybox from flash (sanity)" /rom/bin/busybox true
probe "d) python3, ext4 SD (control)" \
    /opt/lib/ld.so.1 --library-path /opt/lib /opt/bin/python3.13 --version
probe "e) python3, squashfs loop on same SD" \
    $M/lib/ld.so.1 --library-path $M/lib $M/bin/python3.13 --version
echo DONE
