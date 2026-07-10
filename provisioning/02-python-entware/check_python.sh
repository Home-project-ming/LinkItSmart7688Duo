#!/bin/sh
echo "=== arch / endianness ==="
uname -m
echo -n "ELF header of /bin/busybox: "
hexdump -C -n 20 /bin/busybox | head -2

echo
echo "=== libc ==="
opkg list-installed | grep -e '^libc ' -e '^musl' -e '^uclibc'
ls -l /lib/ld* 2>/dev/null
/lib/libc.so 2>&1 | head -3

echo
echo "=== python on board ==="
for p in python python2 python3; do
    if command -v $p >/dev/null 2>&1; then
        printf '%-8s ' "$p"; $p --version 2>&1 | head -1
    else
        printf '%-8s (absent)\n' "$p"
    fi
done

echo
echo "=== FPU? ==="
grep -i -e fpu -e 'ASEs' /proc/cpuinfo

echo
echo "=== memory ==="
free | head -2

echo
echo "=== root space (SD via extroot) ==="
df -h / | tail -1
