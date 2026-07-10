#!/bin/sh
echo "=== PREFLIGHT CHECKLIST ==="

echo
echo "[1] ext4 in /proc/filesystems"
grep -q ext4 /proc/filesystems && echo "  OK" || echo "  FAIL"

echo "[2] crc32c registered in crypto API"
grep -q crc32c /proc/crypto && echo "  OK" || echo "  FAIL"

echo "[3] boot-time modules staged"
for m in 30-fs-ext4 04-crypto-crc32c 02-crypto-hash 20-lib-crc16 mmc sdhci sdhci-mt7620; do
    if [ -e "/etc/modules-boot.d/$m" ]; then echo "  OK   $m"; else echo "  FAIL $m"; fi
done

echo "[4] fstab config on internal overlay (read by preinit)"
grep -q 'a8a9d5f7-91ee-492f-b6c6-b16422b65f7a' /overlay/upper/etc/config/fstab && echo "  OK" || echo "  FAIL"

echo "[5] SD UUID still matches"
/sbin/block info | grep -q 'a8a9d5f7-91ee-492f-b6c6-b16422b65f7a' && echo "  OK" || echo "  FAIL"

echo "[6] overlay data copied to SD"
[ -d /mnt/sd/upper ] && [ -d /mnt/sd/work ] && echo "  OK" || echo "  FAIL"

echo "[7] rootfs (for .extroot-uuid pairing) detectable"
/sbin/block info | grep mtdblock5 >/dev/null && echo "  OK" || echo "  FAIL"

echo
echo "=== current space (BEFORE extroot) ==="
df -h / /overlay | grep -v ^Filesystem

echo
echo "=== unmount /mnt/sd cleanly ==="
sync
umount /mnt/sd && echo "unmounted" || echo "umount failed"
mount | grep mmc || echo "(no mmc mounts - clean)"
