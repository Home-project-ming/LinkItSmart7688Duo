#!/bin/sh
# Quantify: how often do Entware (glibc) binaries die, vs busybox (uClibc)?

trial() {   # trial <label> <n> <cmd...>
    label="$1"; n="$2"; shift 2
    ill=0; seg=0; ok=0; other=0
    i=1
    while [ $i -le $n ]; do
        "$@" >/dev/null 2>&1
        rc=$?
        case $rc in
            0)   ok=$((ok+1)) ;;
            132) ill=$((ill+1)) ;;
            139) seg=$((seg+1)) ;;
            *)   other=$((other+1)) ;;
        esac
        i=$((i+1))
    done
    printf '  %-38s ok=%-3s SIGILL=%-3s SIGSEGV=%-3s other=%s\n' "$label" "$ok" "$ill" "$seg" "$other"
}

echo "=== Entware (glibc 2.27) binaries, 15 runs each ==="
trial "/opt/bin/find /tmp -maxdepth 1"  15 /opt/bin/find /tmp -maxdepth 1
trial "/opt/bin/grep root /etc/passwd"  15 /opt/bin/grep root /etc/passwd
trial "/opt/bin/python3 --version"      15 /opt/bin/python3 --version
trial "/opt/bin/python3 -c 'import ssl'" 15 /opt/bin/python3 -c "import ssl"

echo
echo "=== busybox (uClibc) equivalents, 15 runs each ==="
trial "busybox find /tmp -maxdepth 1"   15 busybox find /tmp -maxdepth 1
trial "busybox grep root /etc/passwd"   15 busybox grep root /etc/passwd

echo
echo "=== kernel messages during/after ==="
dmesg | tail -12

echo
echo "=== memory + swap ==="
free
echo "--- swap configured? ---"
cat /proc/swaps

echo
echo "=== does the loader do anything odd? (rdhwr / TLS is a classic MIPS SIGILL) ==="
echo "--- kernel rdhwr/ULR support ---"
grep -i -e rdhwr -e ulri /proc/cpuinfo 2>/dev/null || echo "  (nothing in cpuinfo)"
echo "--- Entware loader ---"
ls -l /opt/lib/ld.so.1 2>/dev/null
