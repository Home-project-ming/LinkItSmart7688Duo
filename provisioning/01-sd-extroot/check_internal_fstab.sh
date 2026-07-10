#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Mount the INTERNAL jffs2 overlay read-only and inspect the fstab that
# preinit actually reads when deciding whether to do extroot.

echo "=== internal overlay device ==="
/sbin/block info | grep mtdblock6

echo
echo "=== mount /dev/mtdblock6 read-only at /tmp/intovl ==="
mkdir -p /tmp/intovl
mount -t jffs2 -o ro /dev/mtdblock6 /tmp/intovl 2>&1 && echo "mounted ro" || { echo "mount failed"; exit 1; }

echo
echo "=== internal fstab (AUTHORITATIVE for boot-time extroot) ==="
cat /tmp/intovl/upper/etc/config/fstab 2>&1 || echo "(absent)"

echo
echo "=== SD fstab (what you see at /etc/config/fstab while running) ==="
cat /etc/config/fstab 2>&1

echo
echo "=== identical? ==="
if cmp -s /tmp/intovl/upper/etc/config/fstab /etc/config/fstab; then
    echo "  YES - both copies agree"
else
    echo "  NO - they differ (boot uses the internal one)"
fi

echo
echo "=== cleanup ==="
umount /tmp/intovl && rmdir /tmp/intovl && echo "unmounted"

echo
echo "=== sanity: extroot still fine ==="
mount | grep -c mmcblk0p1
df -h / | tail -1
