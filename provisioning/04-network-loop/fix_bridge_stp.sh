#!/bin/sh
# br-lan bridges eth0 + apcli0, both reaching the same upstream router, with STP
# off - that's a guaranteed broadcast storm whenever both paths are up at once.
# This finds the actual UCI bridge section (doesn't assume it's called 'lan')
# and turns STP on so the redundant path gets blocked instead of looped.

echo "=== before ==="
uci show network

SEC=$(uci show network | grep "\.type='bridge'" | head -n1 | cut -d. -f2)
if [ -z "$SEC" ]; then
    echo
    echo "No UCI section with type='bridge' found - not touching anything."
    exit 1
fi

echo
echo "=== bridge section: $SEC ==="
uci show "network.$SEC"

echo
echo "=== applying option stp '1' to network.$SEC ==="
uci set "network.$SEC.stp=1"
uci changes network
uci commit network
/etc/init.d/network reload

sleep 2
echo
echo "=== after ==="
printf 'stp_state: '
cat /sys/class/net/br-lan/bridge/stp_state 2>/dev/null || echo "(br-lan not found)"
uci show "network.$SEC"
