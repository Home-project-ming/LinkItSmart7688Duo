#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
echo "=== install crc32c modules ==="
opkg install /tmp/kmod-crypto-crc32c_3.18.23-1_ramips_24kec.ipk 2>&1
opkg install /tmp/kmod-lib-crc32c_3.18.23-1_ramips_24kec.ipk 2>&1

echo
echo "=== crc32c registered in crypto API? ==="
grep -A2 -e "name.*crc32c" /proc/crypto | head -12

echo
echo "=== mount (rw, real) ==="
mkdir -p /mnt/sd
mount -t ext4 /dev/mmcblk0p1 /mnt/sd
echo "mount rc=$?"

echo
echo "=== mount table ==="
mount | grep mmc || echo NOT_MOUNTED

echo
echo "=== capacity ==="
df -h /mnt/sd 2>&1

echo
echo "=== contents ==="
ls -la /mnt/sd 2>&1 | head -20

echo
echo "=== write test ==="
echo "hello-from-claude" > /mnt/sd/.writetest 2>&1 && cat /mnt/sd/.writetest && rm -f /mnt/sd/.writetest && echo "WRITE_OK"

echo
echo "=== dmesg tail ==="
dmesg | tail -6
