#!/bin/sh
echo "=== before ==="
mount | grep -c mmcblk0p1
mount | grep mmcblk0p1

echo
echo "=== disable + stop mountd ==="
/etc/init.d/mountd disable 2>&1
/etc/init.d/mountd stop 2>&1
sleep 2
ps w | grep -e mountd | grep -v grep || echo "mountd not running"

echo
echo "=== rc.d check (S80mountd should be gone) ==="
ls /etc/rc.d/ | grep -i mountd || echo "no mountd in rc.d - OK"

echo
echo "=== umount stray mount if still present ==="
if mount | grep -q '/tmp/run/mountd/mmcblk0p1'; then
    umount /tmp/run/mountd/mmcblk0p1 2>&1 && echo "unmounted" || echo "umount failed"
else
    echo "(already gone)"
fi

echo
echo "=== remove dangling /Media/SD-P1 symlink ==="
if [ -L /Media/SD-P1 ]; then
    rm -f /Media/SD-P1 && echo "removed"
else
    echo "(no symlink)"
fi
ls -la /Media/ 2>/dev/null

echo
echo "=== after: mmcblk0p1 mount count (want 1) ==="
mount | grep -c mmcblk0p1
mount | grep mmcblk0p1

echo
echo "=== extroot still healthy? ==="
mount | grep -q '/dev/mmcblk0p1 on /overlay' && echo "  extroot OK" || echo "  EXTROOT BROKEN"
df -h /overlay | tail -1

echo
echo "=== write-persistence test ==="
echo "extroot-test-$(date +%s)" > /root/.extroot_test
sync
cat /root/.extroot_test
echo "(file lives on SD via overlay upperdir)"
ls -l /overlay/upper/root/.extroot_test 2>&1
