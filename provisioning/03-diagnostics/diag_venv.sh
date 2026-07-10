#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
export PATH=/opt/bin:/opt/sbin:$PATH

echo "=== did the package actually install? ==="
python3 -c "import virtualenv, sys; print('virtualenv', virtualenv.__version__, 'importable')" 2>&1

echo
echo "=== console script present? ==="
ls -l /opt/bin/virtualenv 2>&1

echo
echo "=== pip sees it? ==="
pip3 show virtualenv 2>&1 | head -4

echo
echo "=== run as module (bypasses console script) ==="
python3 -m virtualenv --version 2>&1 | head -3
echo "rc=$?"

echo
echo "=== which step raises SIGILL? try creating an env via module ==="
python3 -m virtualenv /tmp/ve2 2>&1 | tail -6
echo "rc=$?"
ls -l /tmp/ve2/bin/ 2>/dev/null | head

echo
echo "=== dmesg for the fault ==="
dmesg | tail -8

echo
echo "=== does python-discovery import cleanly? (suspect) ==="
python3 -c "import python_discovery" 2>&1 | tail -2
python3 -c "import distlib, filelock, platformdirs; print('deps import OK')" 2>&1 | tail -2

rm -rf /tmp/ve2
