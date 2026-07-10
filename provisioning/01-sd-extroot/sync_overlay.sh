#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Re-sync /overlay -> SD so the SD copy includes /etc/config/fstab and S40fstab.
set -e

echo "=== re-sync /overlay -> /mnt/sd ==="
mkdir -p /tmp/cproot
mount --bind /overlay /tmp/cproot
tar -C /tmp/cproot -cf - . | tar -C /mnt/sd -xf -
sync
umount /tmp/cproot
rmdir /tmp/cproot
echo "done"

echo
echo "=== verify fstab landed on SD ==="
echo "--- /mnt/sd/upper/etc/config/fstab ---"
cat /mnt/sd/upper/etc/config/fstab 2>&1

echo
echo "--- S40fstab symlink on SD ---"
ls -l /mnt/sd/upper/etc/rc.d/ 2>/dev/null | grep fstab || echo "(missing)"

echo
echo "=== compare internal vs SD (should be identical) ==="
if cmp -s /etc/config/fstab /mnt/sd/upper/etc/config/fstab; then
    echo "IDENTICAL"
else
    echo "DIFFER !!"
fi

echo
echo "=== ext4 modules present on SD copy (for reference) ==="
ls /mnt/sd/upper/etc/modules-boot.d/ 2>/dev/null | grep -e ext4 -e crc32c || echo "(none - they live in /rom or internal upper)"

echo
echo "=== internal overlay fstab (authoritative for preinit) ==="
cat /overlay/upper/etc/config/fstab 2>&1
