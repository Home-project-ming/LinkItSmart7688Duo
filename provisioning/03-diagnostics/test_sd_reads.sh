#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Hypothesis: SD reads are intermittently corrupt, so demand-paged /opt binaries
# execute garbage (SIGILL) once their pages are evicted.
# Test: drop caches, re-read the same bytes, compare checksums.

TARGET=/opt/lib/libpython3.13.so.1.0
[ -f "$TARGET" ] || TARGET=$(ls -S /opt/lib/libpython3.13* 2>/dev/null | head -1)
[ -f "$TARGET" ] || TARGET=/opt/bin/python3.13

echo "=== target file ==="
ls -l "$TARGET"

echo
echo "=== A. filesystem-level: md5 of the same file, cache dropped each time ==="
i=1
while [ $i -le 8 ]; do
    sync
    echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
    h=$(busybox md5sum "$TARGET" | cut -d' ' -f1)
    echo "  pass $i: $h"
    i=$((i + 1))
done

echo
echo "=== B. block-level: md5 of the first 48 MiB of the raw partition ==="
i=1
while [ $i -le 4 ]; do
    sync
    echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
    h=$(busybox dd if=/dev/mmcblk0p1 bs=1M count=48 2>/dev/null | busybox md5sum | cut -d' ' -f1)
    echo "  pass $i: $h"
    i=$((i + 1))
done

echo
echo "=== C. run an Entware binary right after dropping caches (forces re-read) ==="
ill=0; seg=0; ok=0
i=1
while [ $i -le 10 ]; do
    sync
    echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
    /opt/bin/python3 --version >/dev/null 2>&1
    rc=$?
    case $rc in
        0)   ok=$((ok+1)) ;;
        132) ill=$((ill+1)) ;;
        139) seg=$((seg+1)) ;;
    esac
    i=$((i + 1))
done
echo "  python3 --version after drop_caches x10 -> ok=$ok SIGILL=$ill SIGSEGV=$seg"

echo
echo "=== D. same, for busybox (lives on flash/squashfs - control group) ==="
ill=0; seg=0; ok=0
i=1
while [ $i -le 10 ]; do
    sync
    echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
    busybox find /tmp -maxdepth 1 >/dev/null 2>&1
    rc=$?
    case $rc in
        0)   ok=$((ok+1)) ;;
        132) ill=$((ill+1)) ;;
        139) seg=$((seg+1)) ;;
    esac
    i=$((i + 1))
done
echo "  busybox find after drop_caches x10 -> ok=$ok SIGILL=$ill SIGSEGV=$seg"

echo
echo "=== E. mmc / ext4 errors in kernel log? ==="
dmesg | grep -i -e mmc -e ext4 -e 'I/O error' -e 'buffer' | tail -8 || echo "  (none)"

echo
echo "=== F. memory ==="
free | sed -n '2,3p'
