#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
echo "=== /opt state ==="
if [ -e /opt ]; then ls -ld /opt; readlink /opt 2>/dev/null; else echo "(/opt does not exist - good, installer will create it)"; fi

echo
echo "=== is / really the SD (extroot)? ==="
df -h / | tail -1
mount | grep ' / ' | head -2

echo
echo "=== plain-HTTP wget works? ==="
if wget -q -O /tmp/httptest http://bin.entware.net/mipselsf-k3.4/installer/opkg.conf 2>&1; then
    echo "HTTP_OK"; wc -c < /tmp/httptest; head -3 /tmp/httptest; rm -f /tmp/httptest
else
    echo "HTTP_FAIL"
fi

echo
echo "=== current PATH ==="
echo "$PATH"

echo
echo "=== /etc/profile PATH line ==="
grep -n PATH /etc/profile 2>/dev/null

echo
echo "=== /etc/rc.local ==="
cat /etc/rc.local 2>/dev/null

echo
echo "=== free RAM ==="
free | sed -n '2p'
