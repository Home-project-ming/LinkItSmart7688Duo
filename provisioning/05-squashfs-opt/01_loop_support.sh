#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Enable loop-device support for the squashfs /opt experiment.
# Prereq: the three ipks from ./ipk/ have just been scp'd to /tmp
# (do NOT let time pass in between - /tmp is tmpfs and gets cleared).
# kmod-loop vermagic matches the board (3.18.23-1-32322c69...), same
# feed the extroot kmods came from.
opkg install /tmp/kmod-loop_3.18.23-1_ramips_24kec.ipk 2>&1
opkg install /tmp/libsmartcols_2.25.2-4_ramips_24kec.ipk 2>&1
opkg install /tmp/losetup_2.25.2-4_ramips_24kec.ipk 2>&1
insmod loop 2>/dev/null
echo "--- /proc/devices:"
grep -i loop /proc/devices || echo "NO loop driver"
# devtmpfs should create the nodes; mknod as fallback
[ -b /dev/loop0 ] || mknod /dev/loop0 b 7 0
echo "--- loop nodes:"
ls /dev/loop* 2>/dev/null
echo "--- losetup:"
losetup --version
echo DONE
