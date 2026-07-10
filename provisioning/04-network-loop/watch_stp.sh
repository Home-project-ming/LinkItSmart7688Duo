#!/bin/sh
# Poll br-lan's per-port STP state straight from sysfs every 2s (no brctl
# dependency - it may not be installed). State values per the kernel bridge
# driver: 0=disabled 1=listening 2=learning 3=forwarding 4=blocking.
# A working STP fix looks like: one port sits at 4 (blocking) once converged.
while true; do
    date
    for p in /sys/class/net/br-lan/brif/*/state; do
        port=$(basename "$(dirname "$p")")
        printf '  %-8s state=%s\n' "$port" "$(cat "$p" 2>/dev/null)"
    done
    echo "---"
    sleep 2
done
