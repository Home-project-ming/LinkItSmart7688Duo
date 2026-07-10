#!/bin/sh
echo "=== block-mount deps present? ==="
opkg list-installed | grep -e '^ubox ' -e '^libubox ' -e '^libuci ' -e '^libc ' -e '^block-mount '
echo "(end)"

echo
echo "=== existing fstab config ==="
cat /etc/config/fstab 2>/dev/null || echo "(no /etc/config/fstab)"

echo
echo "=== overlay layout ==="
ls -la /overlay/

echo
echo "=== overlay usage ==="
du -sh /overlay/ 2>/dev/null

echo
echo "=== mountd init script ==="
ls -l /etc/rc.d/ | grep -i mountd || echo "(mountd not in rc.d)"

echo
echo "=== is 'block' command present? ==="
command -v block || echo "(no block command - block-mount not installed)"
