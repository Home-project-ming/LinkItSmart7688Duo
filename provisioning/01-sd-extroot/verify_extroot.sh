#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
echo "=== uptime (confirm we really rebooted) ==="
uptime

echo
echo "=== IS EXTROOT ACTIVE? ==="
if mount | grep -q '/dev/mmcblk0p1 on /overlay'; then
    echo "  *** YES - /overlay is on the SD card ***"
else
    echo "  *** NO - still on internal flash ***"
fi

echo
echo "=== mount table ==="
mount | grep -e overlay -e mmc -e mtdblock

echo
echo "=== df -h ==="
df -h

echo
echo "=== overlayfs upperdir source ==="
grep -e overlay /proc/mounts

echo
echo "=== .extroot-uuid pairing file (auto-created by block) ==="
cat /overlay/etc/.extroot-uuid 2>/dev/null && echo "" || echo "(not at /overlay/etc/.extroot-uuid)"
find / -maxdepth 4 -name '.extroot-uuid' 2>/dev/null

echo
echo "=== rootfs uuid for comparison ==="
/sbin/block info | grep mtdblock5

echo
echo "=== mountd conflict check (is SD double-mounted?) ==="
mount | grep -c mmcblk0p1
ls -l /Media/ 2>/dev/null
ls /tmp/run/mountd/ 2>/dev/null || echo "(mountd dir empty)"

echo
echo "=== dmesg: extroot / ext4 / mmc lines ==="
dmesg | grep -i -e extroot -e ext4 -e mmc -e overlay | head -20

echo
echo "=== free space gained ==="
df -h /overlay | tail -1
