#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Install Entware's squashfs-tools (mksquashfs) on the board.
# Entware's opkg is itself a glibc binary living on the SD card, i.e.
# subject to the very cold-cache SIGILL bug this experiment targets -
# so every invocation gets a retry loop.

retry() {
    n=1
    while [ $n -le 6 ]; do
        "$@" && return 0
        echo "  (attempt $n failed rc=$?, retrying)"
        n=$((n+1))
        sleep 1
    done
    echo "FAILED after 6 attempts: $*"
    return 1
}

echo "--- opkg update:"
retry /opt/bin/opkg update

echo "--- available squashfs packages:"
retry /opt/bin/opkg list 'squashfs*'

echo "--- install:"
# Entware splits squashfs-tools into per-tool packages
# (squashfs-tools-mksquashfs / squashfs-tools-unsquashfs)
retry /opt/bin/opkg install squashfs-tools-mksquashfs

echo "--- result:"
ls -la /opt/sbin/mksquashfs /opt/bin/mksquashfs 2>/dev/null
retry sh -c 'PATH=/opt/sbin:/opt/bin:$PATH mksquashfs -version | head -2'
echo DONE
