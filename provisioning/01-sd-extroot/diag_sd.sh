#!/bin/sh
# Diagnose why mounting the ext4 SD card hangs.

echo "=== ext in /proc/filesystems ==="
grep ext /proc/filesystems

echo
echo "=== ext4-related modules loaded ==="
lsmod | grep -e ext4 -e jbd2 -e mbcache -e crc16 -e crypto

echo
echo "=== stale mounts from earlier attempts ==="
mount | grep -e mmc -e /mnt/sd || echo "(none)"

echo
echo "=== raw read sanity: 4 KiB from partition start ==="
dd if=/dev/mmcblk0p1 of=/dev/null bs=4096 count=1 2>&1

echo
echo "=== raw read sanity: 1 MiB at 64 GiB offset ==="
dd if=/dev/mmcblk0p1 of=/dev/null bs=1M skip=65536 count=1 2>&1

echo
echo "=== try: ro,noload (skips journal replay) ==="
mkdir -p /mnt/sd
mount -t ext4 -o ro,noload /dev/mmcblk0p1 /mnt/sd &
MPID=$!
i=0
while [ $i -lt 15 ]; do
    kill -0 $MPID 2>/dev/null || break
    sleep 1
    i=$((i + 1))
done

if kill -0 $MPID 2>/dev/null; then
    echo "RESULT: still hanging after ${i}s -> killing"
    kill -9 $MPID 2>/dev/null
else
    wait $MPID
    echo "RESULT: mount returned rc=$?  (after ${i}s)"
fi

echo
echo "=== mount table ==="
mount | grep mmc || echo NOT_MOUNTED

echo
echo "=== listing (if mounted) ==="
ls -la /mnt/sd 2>&1 | head -15

echo
echo "=== dmesg tail ==="
dmesg | tail -15
