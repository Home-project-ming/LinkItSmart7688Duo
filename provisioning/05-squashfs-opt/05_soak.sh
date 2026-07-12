#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Soak test after the 10x probe succeeded: 20 more cold-cache rounds
# each for grep and python3 from the squashfs mount, plus a real Python
# computation (not just --version) with its stdlib resolved from inside
# the image. Target: zero failures across all rounds.
M=/mnt/opt-sq
mountpoint -q $M 2>/dev/null || mount | grep -q "$M" || {
    losetup /dev/loop0 /overlay/opt.sqfs 2>/dev/null
    mount -t squashfs -o ro /dev/loop0 $M || exit 1
}

probe() {
    label="$1"; rounds="$2"; shift 2
    ok=0; fail=0
    i=1
    while [ $i -le $rounds ]; do
        sync
        echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
        "$@" >/dev/null 2>&1
        [ $? -eq 0 ] && ok=$((ok+1)) || fail=$((fail+1))
        i=$((i + 1))
    done
    printf '  %-44s ok=%-3s fail=%s\n' "$label" "$ok" "$fail"
}

echo "=== soak: cold cache every round ==="
probe "grep --version (squashfs) x20" 20 \
    $M/lib/ld.so.1 --library-path $M/lib $M/bin/grep --version
probe "python3 --version (squashfs) x20" 20 \
    $M/lib/ld.so.1 --library-path $M/lib $M/bin/python3.13 --version
probe "python3 real compute (squashfs) x10" 10 \
    env PYTHONHOME=$M $M/lib/ld.so.1 --library-path $M/lib \
    $M/bin/python3.13 -I -c 'print(sum(range(100000)))'
echo DONE
