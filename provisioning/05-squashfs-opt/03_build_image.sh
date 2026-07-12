#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Build /overlay/opt.sqfs from the live /opt tree.
# mksquashfs is itself an Entware glibc binary on the SD card - the very
# thing that crashes on cold-cache execution - so stage it (plus the
# Entware loader and libs) into tmpfs and run it from RAM: the pattern
# the original SIGILL investigation proved reliable (case b, 10/10).
# Reading the /opt source files and writing the image use plain
# read()/write(), which the investigation showed are bit-perfect - only
# demand-paged *execution* from SD is broken.
E=/tmp/mksq
rm -rf $E; mkdir -p $E
cp /opt/lib/ld-2.27.so $E/ 2>/dev/null || cp /opt/lib/ld.so.1 $E/ld-2.27.so
for f in /opt/lib/*.so*; do cp -a "$f" $E/ 2>/dev/null; done
cp /opt/sbin/mksquashfs $E/
chmod 755 $E/mksquashfs $E/ld-2.27.so
sync
echo "--- staged $(busybox du -sh $E | busybox cut -f1) into tmpfs, building..."
rm -f /overlay/opt.sqfs
# -comp xz is NOT a choice: this kernel's squashfs was built with only
# the xz decompressor (mounting a gzip image fails with dmesg
# 'Filesystem uses "zlib" compression. This is not supported' - the
# stock rootfs is squashfs-xz, so that's all OpenWrt compiled in).
$E/ld-2.27.so --library-path $E $E/mksquashfs /opt /overlay/opt.sqfs \
    -comp xz -b 131072 -noappend -quiet
echo "mksquashfs rc=$?"
ls -la /overlay/opt.sqfs
rm -rf $E
echo DONE
