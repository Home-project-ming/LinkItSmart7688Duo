#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Idempotently installs the squashfs /opt boot-mount hook into /etc/rc.local.
# See docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md
MARKER="# squashfs /opt boot mount"
RC=/etc/rc.local

if grep -qF "$MARKER" "$RC"; then
    echo "ALREADY_INSTALLED"
    exit 0
fi

TMP=/tmp/rc.local.new
grep -v '^exit 0$' "$RC" > "$TMP"
cat >> "$TMP" <<'HOOK'
# squashfs /opt boot mount - see docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md
if ! mount | grep -q ' /opt '; then
    if [ -f /overlay/opt.sqfs ]; then
        losetup /dev/loop0 /overlay/opt.sqfs 2>/tmp/opt-sqfs-boot.log \
            && mount -t squashfs -o ro /dev/loop0 /opt 2>>/tmp/opt-sqfs-boot.log \
            || logger -t opt-sqfs "boot mount failed, /opt stays writable - see /tmp/opt-sqfs-boot.log"
    else
        logger -t opt-sqfs "/overlay/opt.sqfs missing, /opt stays writable"
    fi
fi

exit 0
HOOK

cp "$RC" "$RC.bak"
mv "$TMP" "$RC"
chmod 755 "$RC"
echo "INSTALLED"
cat "$RC"
