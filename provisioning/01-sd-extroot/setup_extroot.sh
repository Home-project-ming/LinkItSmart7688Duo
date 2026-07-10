#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Configure extroot: move the OpenWrt overlay onto the SD card.
# Safe to re-run. Does NOT reboot.
set -e

UUID='a8a9d5f7-91ee-492f-b6c6-b16422b65f7a'
DEV='/dev/mmcblk0p1'

echo "=== 1. ensure SD is mounted ==="
mkdir -p /mnt/sd
if ! mount | grep -q ' /mnt/sd '; then
    mount -t ext4 "$DEV" /mnt/sd
fi
mount | grep /mnt/sd

echo
echo "=== 2. copy /overlay -> SD (bind mount avoids recursion) ==="
mkdir -p /tmp/cproot
mount --bind /overlay /tmp/cproot
tar -C /tmp/cproot -cf - . | tar -C /mnt/sd -xf -
sync
umount /tmp/cproot
rmdir /tmp/cproot

echo
echo "=== 3. verify copy ==="
ls -la /mnt/sd/
echo "--- upper/ present? ---"
ls /mnt/sd/upper >/dev/null 2>&1 && echo "upper/ OK" || echo "upper/ MISSING"
echo "--- work/ present? ---"
ls -d /mnt/sd/work >/dev/null 2>&1 && echo "work/ OK" || echo "work/ MISSING"
echo "--- size on SD ---"
du -sh /mnt/sd/upper 2>/dev/null

echo
echo "=== 4. write /etc/config/fstab ==="
cat > /etc/config/fstab <<EOF
config global
	option anon_swap '0'
	option anon_mount '0'
	option auto_swap '1'
	option auto_mount '1'
	option delay_root '10'
	option check_fs '0'

config mount
	option target '/overlay'
	option uuid '$UUID'
	option enabled '1'
EOF
cat /etc/config/fstab

echo
echo "=== 5. enable fstab init script ==="
/etc/init.d/fstab enable 2>&1 || true
ls -l /etc/rc.d/ | grep fstab || echo "(fstab not in rc.d)"

echo
echo "=== 6. sanity: uuid matches device ==="
/sbin/block info | grep mmcblk0p1

echo
echo "=== DONE - not rebooted ==="
