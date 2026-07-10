#!/bin/sh
# READ-ONLY verification. Modifies nothing outside /tmp.
export PATH=/opt/bin:/opt/sbin:$PATH

echo "=== interpreter + ssl ==="
python3 -c "import sys, ssl; print('python', sys.version.split()[0]); print('openssl', ssl.OPENSSL_VERSION)"

echo
echo "=== REAL TEST: pip install a pure-python package from PyPI ==="
echo "(installing into /tmp/pt so nothing persists)"
pip3 install --no-cache-dir --disable-pip-version-check --target /tmp/pt six 2>&1 | tail -5

echo
echo "=== import it ==="
PYTHONPATH=/tmp/pt python3 -c "import six; print('six', six.__version__, 'OK')"

echo
echo "=== venv support? ==="
if python3 -m venv /tmp/venvtest 2>/tmp/venverr; then
    echo "  venv OK"
    /tmp/venvtest/bin/python3 --version
else
    echo "  venv FAILED:"; head -3 /tmp/venverr
fi

echo
echo "=== cleanup ==="
rm -rf /tmp/pt /tmp/venvtest /tmp/venverr
echo done

echo
echo "=== footprint ==="
du -sh /opt
df -h / | tail -1

echo
echo "=== current state of files I would need to touch ==="
echo "--- /etc/profile (PATH line) ---"
grep -n PATH /etc/profile
echo "--- /etc/rc.local ---"
cat /etc/rc.local
