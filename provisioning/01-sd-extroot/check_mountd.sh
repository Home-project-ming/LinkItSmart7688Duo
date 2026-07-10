#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
echo "=== /etc/config/mountd ==="
cat /etc/config/mountd 2>/dev/null || echo "(no uci config)"

echo
echo "=== mountd init script ==="
cat /etc/init.d/mountd 2>/dev/null

echo
echo "=== mountd options (strings) ==="
strings /sbin/mountd 2>/dev/null | grep -i -e blacklist -e ignore -e mmcblk -e '^-' | head -20

echo
echo "=== what package owns mountd ==="
opkg search /sbin/mountd 2>/dev/null

echo
echo "=== who uses /Media ==="
grep -rsl '/Media' /etc/ /www/ 2>/dev/null | head -10
