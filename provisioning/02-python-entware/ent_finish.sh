#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Wire Entware into PATH + boot, then prove pip can reach PyPI.

echo "=== 1. PATH in /etc/profile ==="
if grep -q '/opt/bin' /etc/profile; then
    echo "  already present"
else
    printf '\n# Entware\nexport PATH=/opt/bin:/opt/sbin:$PATH\n' >> /etc/profile
    echo "  appended"
fi
grep -n 'opt/bin' /etc/profile

echo
echo "=== 2. start Entware services at boot (/etc/rc.local) ==="
if grep -q 'rc.unslung' /etc/rc.local; then
    echo "  already present"
else
    sed -i 's|^exit 0|[ -x /opt/etc/init.d/rc.unslung ] \&\& /opt/etc/init.d/rc.unslung start\n\nexit 0|' /etc/rc.local
    echo "  inserted before exit 0"
fi
cat /etc/rc.local

echo
echo "=== 3. sanity: interpreter + ssl module ==="
export PATH=/opt/bin:/opt/sbin:$PATH
python3 -c "import sys, ssl; print('python', sys.version.split()[0]); print('openssl', ssl.OPENSSL_VERSION)"

echo
echo "=== 4. REAL TEST: pip install a pure-python package from PyPI ==="
pip3 install --no-cache-dir --disable-pip-version-check six 2>&1 | tail -5

echo
echo "=== 5. does it import? ==="
python3 -c "import six; print('six', six.__version__, 'from', six.__file__)"

echo
echo "=== 6. venv support? ==="
python3 -m venv /tmp/venvtest 2>&1 | tail -3 && echo "  venv OK" || echo "  venv unavailable"
[ -x /tmp/venvtest/bin/python3 ] && /tmp/venvtest/bin/python3 --version
rm -rf /tmp/venvtest

echo
echo "=== 7. footprint ==="
du -sh /opt
df -h / | tail -1
