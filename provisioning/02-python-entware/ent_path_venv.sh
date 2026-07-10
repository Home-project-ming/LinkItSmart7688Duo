#!/bin/sh
# Approved: append Entware to PATH in /etc/profile. Does NOT touch /etc/rc.local.

echo "=== 1. /etc/profile before ==="
grep -n PATH /etc/profile

echo
echo "=== 2. append Entware PATH (idempotent) ==="
if grep -q '/opt/bin' /etc/profile; then
    echo "  already present, skipping"
else
    printf '\n# Entware\nexport PATH=/opt/bin:/opt/sbin:$PATH\n' >> /etc/profile
    echo "  appended"
fi

echo
echo "=== 3. /etc/profile after ==="
tail -4 /etc/profile

echo
echo "=== 4. install virtualenv (venv replacement) ==="
export PATH=/opt/bin:/opt/sbin:$PATH
pip3 install --no-cache-dir --disable-pip-version-check virtualenv 2>&1 | tail -4

echo
echo "=== 5. prove virtualenv builds a working env ==="
cd /tmp
virtualenv /tmp/ve 2>&1 | tail -3
if [ -x /tmp/ve/bin/python ]; then
    /tmp/ve/bin/python --version
    /tmp/ve/bin/pip --version 2>&1 | head -1
    echo "  VIRTUALENV OK"
else
    echo "  VIRTUALENV FAILED"
fi
rm -rf /tmp/ve

echo
echo "=== 6. footprint ==="
du -sh /opt
df -h / | tail -1
