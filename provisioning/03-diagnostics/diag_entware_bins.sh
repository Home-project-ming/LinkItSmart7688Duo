#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Is the SIGSEGV a virtualenv problem, or are Entware binaries generally unstable?

run() {   # run() <label> <cmd...>
    label="$1"; shift
    "$@" >/dev/null 2>&1
    rc=$?
    case $rc in
        0)   r="ok" ;;
        132) r="SIGILL" ;;
        139) r="SIGSEGV" ;;
        *)   r="rc=$rc" ;;
    esac
    printf '  %-34s %s\n' "$label" "$r"
}

echo "=== Entware coreutils vs busybox ==="
run "/opt/bin/find /tmp -maxdepth 1"      /opt/bin/find /tmp -maxdepth 1
run "/opt/bin/find /opt -name '*.pyc'"    /opt/bin/find /opt -name '*.pyc'
run "/opt/bin/grep -r root /etc/passwd"   /opt/bin/grep -r root /etc/passwd
run "busybox find /tmp -maxdepth 1"       busybox find /tmp -maxdepth 1
run "busybox grep root /etc/passwd"       busybox grep root /etc/passwd

echo
echo "=== Entware find, deeper tree (memory / recursion) ==="
run "/opt/bin/find /opt -type f"          /opt/bin/find /opt -type f

echo
echo "=== python3 basics ==="
run "python3 --version"                   /opt/bin/python3 --version
run "python3 -c pass"                     /opt/bin/python3 -c pass
run "python3 -c import-many"              /opt/bin/python3 -c "import json,ssl,sqlite3,ctypes,lzma,bz2"

echo
echo "=== virtualenv: which phase segfaults? ==="
export PATH=/opt/bin:/opt/sbin:$PATH

rm -rf /tmp/vA /tmp/vB
echo "--- (1) --no-seed  (skip installing pip into the env) ---"
python3 -m virtualenv --no-seed /tmp/vA >/tmp/vA.log 2>&1
echo "  rc=$?"
tail -3 /tmp/vA.log
[ -x /tmp/vA/bin/python ] && echo "  interpreter present"

echo
echo "--- (2) default (seeds pip via app-data) ---"
python3 -m virtualenv /tmp/vB >/tmp/vB.log 2>&1
echo "  rc=$?"
echo "  --- full log ---"
cat /tmp/vB.log
[ -x /tmp/vB/bin/pip ] && echo "  pip present in env" || echo "  pip MISSING in env"

echo
echo "=== memory ==="
free | sed -n '2p'

rm -rf /tmp/vA /tmp/vB /tmp/vA.log /tmp/vB.log
