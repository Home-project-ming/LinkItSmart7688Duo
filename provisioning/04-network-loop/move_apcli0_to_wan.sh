#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# apcli0 (the STA/uplink radio) is dual-homed onto 'lan' via option network
# 'lan' on its wifi-iface in /etc/config/wireless - that's what actually put it
# in br-lan (not /etc/config/network's ifname list), and what closes the
# eth0<->apcli0 loop through the upstream router. Move it onto the already
# NAT'd/isolated 'wan' network where it belongs.
#
# Warning: this SSH session is very likely running over apcli0 right now (eth0
# is unplugged). The connection may drop the moment `wifi reload` applies -
# that's expected. Once apcli0 moves off 'lan', eth0 is safe to reconnect
# (no more loop) and is the way back in.

echo "=== before: wireless ==="
uci show wireless

SEC=$(uci show wireless | grep "\.mode='sta'" | head -n1 | cut -d. -f2)
if [ -z "$SEC" ]; then
    echo
    echo "No wifi-iface with mode='sta' found - not touching anything."
    exit 1
fi

echo
echo "=== sta section: $SEC ==="
uci show "wireless.$SEC"

echo
echo "=== before: br-lan members ==="
ls /sys/class/net/br-lan/brif/ 2>/dev/null

echo
echo "=== setting wireless.$SEC.network = wan ==="
uci set "wireless.$SEC.network=wan"
uci changes wireless
uci commit wireless
wifi reload

sleep 5
echo
echo "=== after: br-lan members ==="
ls /sys/class/net/br-lan/brif/ 2>/dev/null
echo
echo "=== after: wireless ==="
uci show "wireless.$SEC"
