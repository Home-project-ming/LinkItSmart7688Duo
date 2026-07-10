#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
export PATH=/opt/bin:/opt/sbin:$PATH

echo "=== A. faulthandler traceback on: import virtualenv ==="
python3 -X faulthandler -c "import virtualenv" 2>&1 | tail -25

echo
echo "=== B. isolate the suspects one by one ==="
for m in ctypes _ctypes importlib.metadata python_discovery distlib filelock platformdirs; do
    printf '%-22s ' "$m"
    if python3 -c "import $m" >/dev/null 2>&1; then echo "ok"; else echo "*** CRASH/FAIL ***"; fi
done

echo
echo "=== C. ctypes deeper (libffi closures are a classic MIPS SIGILL) ==="
python3 -X faulthandler -c "
import ctypes
print('ctypes imported')
libc = ctypes.CDLL(None)
print('CDLL(None) ok')
cb = ctypes.CFUNCTYPE(ctypes.c_int)(lambda: 42)
print('closure created:', cb())
" 2>&1 | tail -15

echo
echo "=== D. last module imported before the crash (verbose import) ==="
python3 -v -c "import virtualenv" 2>&1 | tail -12

echo
echo "=== E. what does python_discovery pull in? ==="
python3 -X faulthandler -c "import python_discovery; print('python_discovery ok')" 2>&1 | tail -12
