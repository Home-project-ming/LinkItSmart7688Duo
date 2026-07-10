#!/bin/sh
echo "=== /etc/modules-boot.d/ ==="
ls -la /etc/modules-boot.d/ 2>/dev/null || echo "(does not exist)"

echo
echo "=== /etc/modules.d/ (ext4 / crc / crypto / mmc related) ==="
ls /etc/modules.d/ | grep -e ext4 -e crc -e crypto -e mmc -e sdhci

echo
echo "=== contents of those files ==="
for f in /etc/modules.d/*ext4* /etc/modules.d/*crc* /etc/modules.d/*crypto* ; do
    [ -f "$f" ] || continue
    echo "--- $f ---"
    cat "$f"
done

echo
echo "=== where does kmodloader run in preinit? ==="
grep -r -l kmodloader /lib/preinit/ 2>/dev/null || echo "(no kmodloader in /lib/preinit)"
echo "--- /etc/init.d/boot kmodloader lines ---"
grep -n kmodloader /etc/init.d/boot 2>/dev/null

echo
echo "=== preinit hooks (ordered) ==="
ls /lib/preinit/

echo
echo "=== does mount_root exist / is it fstools? ==="
command -v mount_root
opkg list-installed | grep -e fstools -e fstab
