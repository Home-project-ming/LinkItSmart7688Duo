#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Robust check: is the mountd daemon actually running?
# (avoid pgrep -f, which matches our own command line)

echo "=== pgrep -x (exact process name) ==="
if pgrep -x mountd >/dev/null 2>&1; then
    echo "  RUNNING (pids: $(pgrep -x mountd | tr '\n' ' '))"
else
    echo "  not running"
fi

echo
echo "=== full process table, mountd entries ==="
ps w | grep '[m]ountd' || echo "  (none)"

echo
echo "=== /proc/*/exe pointing at mountd ==="
found=0
for d in /proc/[0-9]*; do
    exe=$(readlink "$d/exe" 2>/dev/null)
    case "$exe" in
        */mountd) echo "  pid ${d#/proc/} -> $exe"; found=1 ;;
    esac
done
[ "$found" = 0 ] && echo "  (none)"

echo
echo "=== pid file ==="
cat /var/run/mountd.pid 2>/dev/null || echo "  (no pid file)"

echo
echo "=== rc.d ==="
ls /etc/rc.d/ | grep -i mountd || echo "  no mountd startup link - stays disabled across reboots"

echo
echo "=== final: mmcblk0p1 mount count (want exactly 1) ==="
mount | grep -c mmcblk0p1
