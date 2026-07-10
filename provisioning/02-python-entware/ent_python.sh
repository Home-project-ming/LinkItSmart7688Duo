#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
echo "=== Entware opkg works? ==="
/opt/bin/opkg --version 2>&1 | head -1

echo
echo "=== refresh lists ==="
/opt/bin/opkg update 2>&1 | tail -2

echo
echo "=== install python3 + pip ==="
/opt/bin/opkg install python3 python3-pip 2>&1 | grep -e '^Installing' -e '^Configuring' -e 'error' -e 'Error' | tail -40

echo
echo "=== versions ==="
/opt/bin/python3 --version 2>&1
/opt/bin/pip3 --version 2>&1

echo
echo "=== disk used by /opt ==="
du -sh /opt 2>/dev/null
df -h / | tail -1
