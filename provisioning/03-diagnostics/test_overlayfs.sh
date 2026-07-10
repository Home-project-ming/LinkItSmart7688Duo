#!/bin/sh
# Decisive test: the SAME inode, reached two ways.
#   /root/bbtest              -> through overlayfs (upperdir on SD)
#   /overlay/upper/root/bbtest -> the same file, direct on the ext4 mount
# Only the mount/path differs. Data, storage, binary are identical.

BB=/rom/bin/busybox
[ -f "$BB" ] || BB=/bin/busybox

rm -f /root/bbtest
cp "$BB" /root/bbtest
chmod 755 /root/bbtest
sync

OVL=/overlay/upper/root/bbtest

echo "=== same inode? ==="
busybox ls -li /root/bbtest "$OVL" 2>&1
echo "--- md5 via both paths ---"
busybox md5sum /root/bbtest "$OVL"

probe() {
    label="$1"; shift
    ok=0; ill=0; bus=0; seg=0; other=0
    i=1
    while [ $i -le 10 ]; do
        sync
        echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
        "$@" >/dev/null 2>&1
        rc=$?
        case $rc in
            0)   ok=$((ok+1)) ;;
            132) ill=$((ill+1)) ;;
            138) bus=$((bus+1)) ;;
            139) seg=$((seg+1)) ;;
            *)   other=$((other+1)) ;;
        esac
        i=$((i + 1))
    done
    printf '  %-44s ok=%-3s ILL=%-3s BUS=%-3s SEGV=%-3s other=%s\n' \
        "$label" "$ok" "$ill" "$bus" "$seg" "$other"
}

echo
echo "=== busybox, cold cache, 10x from each path ==="
probe "via overlayfs   /root/bbtest"        /root/bbtest true
probe "direct ext4     /overlay/upper/..."  "$OVL" true

echo
echo "=== and an Entware binary reached the direct-ext4 way ==="
if [ -x /overlay/upper/opt/bin/grep ]; then
    probe "via overlayfs   /opt/bin/grep"        /opt/bin/grep --version
    probe "direct ext4     /overlay/upper/opt/bin/grep" /overlay/upper/opt/bin/grep --version
else
    echo "  (/overlay/upper/opt/bin/grep not found; listing what is there)"
    ls /overlay/upper/opt/bin/ 2>&1 | head -5
fi

echo
echo "=== cleanup ==="
rm -f /root/bbtest
sync
echo done
