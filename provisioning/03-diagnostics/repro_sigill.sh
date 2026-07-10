#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
export PATH=/opt/bin:/opt/sbin:$PATH

echo "=== memory before ==="
free | sed -n '2p'

echo
echo "=== A. import virtualenv x5 (exit code captured correctly, no pipeline) ==="
i=1
while [ $i -le 5 ]; do
    python3 -c "import virtualenv" >/dev/null 2>&1
    rc=$?
    case $rc in
        0)   res="ok" ;;
        132) res="SIGILL (128+4)" ;;
        139) res="SIGSEGV (128+11)" ;;
        *)   res="rc=$rc" ;;
    esac
    echo "  run $i: $res"
    i=$((i + 1))
done

echo
echo "=== B. are .pyc caches now present? ==="
find /opt/lib/python3.13/site-packages/virtualenv -name '*.pyc' 2>/dev/null | wc -l
echo "  (count of cached bytecode files)"

echo
echo "=== C. python3 -m virtualenv --version, exit code captured ==="
out=$(python3 -m virtualenv --version 2>&1)
rc=$?
echo "  rc=$rc"
echo "  out: $out" | head -3

echo
echo "=== D. actually build an env, capture rc ==="
rm -rf /tmp/ve3
python3 -m virtualenv /tmp/ve3 >/tmp/ve3.log 2>&1
rc=$?
echo "  rc=$rc"
tail -4 /tmp/ve3.log
echo "  --- resulting env ---"
if [ -x /tmp/ve3/bin/python ]; then
    /tmp/ve3/bin/python --version 2>&1
    echo "  ENV WORKS"
else
    echo "  no usable interpreter"
fi

echo
echo "=== E. memory after ==="
free | sed -n '2p'

echo
echo "=== F. console script missing? reinstall to regenerate ==="
ls -l /opt/bin/virtualenv 2>&1

rm -rf /tmp/ve3 /tmp/ve3.log
