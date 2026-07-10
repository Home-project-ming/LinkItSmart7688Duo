#!/bin/sh
echo "=== who calls kmodloader with modules-boot.d ? ==="
grep -rs 'modules-boot' /etc/ /lib/ /sbin/ /usr/ 2>/dev/null | head -20
echo "(end grep modules-boot)"

echo
echo "=== /etc/preinit ==="
cat /etc/preinit 2>/dev/null

echo
echo "=== /lib/preinit/80_mount_root ==="
cat /lib/preinit/80_mount_root 2>/dev/null

echo
echo "=== strings in kmodloader re boot dir ==="
strings /sbin/kmodloader 2>/dev/null | grep -e modules -e boot | head -10

echo
echo "=== /lib/preinit/03_preinit_do_ramips.sh ==="
cat /lib/preinit/03_preinit_do_ramips.sh 2>/dev/null

echo
echo "=== does mount_root/block reference extroot ==="
strings /sbin/block 2>/dev/null | grep -i -e extroot -e overlay | head -10
