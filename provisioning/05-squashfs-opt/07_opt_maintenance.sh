#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# unlock:  expose the real writable /opt for opkg operations.
# rebuild: rebuild /overlay/opt.sqfs from the current /opt and remount it.
# See docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md
set -e

cmd="$1"

do_unlock() {
    if mount | grep -q ' /opt '; then
        umount /opt
        echo "unmounted /opt"
    else
        echo "/opt was not mounted"
    fi
    if losetup -d /dev/loop0 2>/dev/null; then
        echo "detached /dev/loop0"
    else
        echo "/dev/loop0 was not attached"
    fi
}

do_rebuild() {
    if mount | grep -q ' /opt '; then
        echo "ERROR: /opt is still mounted read-only - run 'unlock' first" >&2
        exit 1
    fi
    E=/tmp/mksq
    rm -rf $E; mkdir -p $E
    cp /opt/lib/ld-2.27.so $E/ 2>/dev/null || cp /opt/lib/ld.so.1 $E/ld-2.27.so
    for f in /opt/lib/*.so*; do cp -a "$f" $E/ 2>/dev/null; done
    cp /opt/sbin/mksquashfs $E/
    chmod 755 $E/mksquashfs $E/ld-2.27.so
    sync
    echo "--- staged $(busybox du -sh $E | busybox cut -f1) into tmpfs, building..."
    rm -f /overlay/opt.sqfs
    $E/ld-2.27.so --library-path $E $E/mksquashfs /opt /overlay/opt.sqfs \
        -comp xz -b 131072 -noappend -quiet
    echo "mksquashfs rc=$?"
    ls -la /overlay/opt.sqfs
    rm -rf $E

    losetup /dev/loop0 /overlay/opt.sqfs
    mount -t squashfs -o ro /dev/loop0 /opt
    echo "remounted /opt from rebuilt image"
    mount | grep ' /opt '
}

case "$cmd" in
    unlock)  do_unlock ;;
    rebuild) do_rebuild ;;
    *)
        echo "usage: $0 {unlock|rebuild}" >&2
        exit 1
        ;;
esac
