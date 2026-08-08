#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
#
# Manual bring-up probe for the XCS20 slave-serial download path.
#
# This is the rung between "fpga_configure --dry-run" (parses the .bit,
# touches no hardware) and a full download (178,144 bits, too fast to
# follow with a meter). Every command here drives ONE thing and stops,
# so a human with a DMM can walk stages 3 and 4 of the design doc's
# bring-up procedure before any Go code runs:
#
#   docs/superpowers/specs/2026-07-30-xcs20-slave-serial-download-design.md
#
# busybox ash only - no bash, no python, nothing from /opt.
#
# Usage (pins are always: din cclk prog gate done):
#   ./01_manual_probe.sh <din> <cclk> <prog> <gate> <done> <command> [args]
#
# Commands:
#   init            export the pins, set directions, park everything safe
#   status          read all five back
#   gate on|off     on = we drive the board (PD3 low); off = buffers Hi-Z
#   set SIG 0|1     drive one of: din cclk prog     (SIG is the name, not a pin)
#   clock N [SEC]   issue N CCLK cycles, optionally SEC seconds per edge
#   pulse-program   the /PROGRAM low-high pulse, then read DONE
#   release         park safe and unexport
#
# Typical session over ssh (see provisioning/README.md for the ssh options):
#   P="39 40 41 42 43"
#   ssh ... "/tmp/01_manual_probe.sh $P init"
#   ssh ... "/tmp/01_manual_probe.sh $P status"
#   ssh ... "/tmp/01_manual_probe.sh $P gate on"
#   ssh ... "/tmp/01_manual_probe.sh $P set cclk 1"     # measure at the 74HC125
#   ssh ... "/tmp/01_manual_probe.sh $P pulse-program"
#   ssh ... "/tmp/01_manual_probe.sh $P release"

set -u

# Overridable so the script can be exercised against a fake sysfs tree on a
# dev machine. Never set this on the board.
SYSFS_GPIO="${SYSFS_GPIO:-/sys/class/gpio}"

if [ $# -lt 6 ]; then
    sed -n '17,36p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
fi

DIN=$1; CCLK=$2; PROG=$3; GATE=$4; DONE=$5
shift 5
CMD=$1
shift

# Mirror of blockedPins in mt7688-mcp-server/blocklist.go, which is the
# source of truth. Kept here because this script deliberately runs before
# any Go code is on the board; if the two ever disagree, blocklist.go wins.
BLOCKED="0 1 2 3 6 7 8 9 10 12 13 18 19 20 21 22 23 24 25 26 27 28 29 38 45 46"

die() { echo "ERROR: $*" >&2; exit 1; }

check_pin() {
    _name=$1; _pin=$2
    case "$_pin" in
        ''|*[!0-9]*) die "$_name: '$_pin' is not a GPIO number" ;;
    esac
    for _b in $BLOCKED; do
        [ "$_pin" = "$_b" ] && die "$_name: GPIO $_pin is on the blocklist (see blocklist.go) - it belongs to the console, SD/extroot, I2S, SPI, the ATmega, or a UART"
    done
}

check_pins() {
    check_pin din "$DIN"; check_pin cclk "$CCLK"; check_pin prog "$PROG"
    check_pin gate "$GATE"; check_pin done "$DONE"
    # Two signals on one pin cannot work and has no electrical symptom.
    _all="$DIN $CCLK $PROG $GATE $DONE"
    for _p in $_all; do
        _n=0
        for _q in $_all; do [ "$_p" = "$_q" ] && _n=$((_n + 1)); done
        [ "$_n" -gt 1 ] && die "GPIO $_p is assigned to more than one signal"
    done
}

export_pin() {
    # Writing an already-exported pin returns EBUSY; that is success here.
    [ -d "$SYSFS_GPIO/gpio$1" ] || echo "$1" > "$SYSFS_GPIO/export" 2>/dev/null
    [ -d "$SYSFS_GPIO/gpio$1" ] || die "could not export GPIO $1 (does this pin exist on this SoC?)"
    echo "$2" > "$SYSFS_GPIO/gpio$1/direction" || die "could not set GPIO $1 direction=$2"
}

put() { echo "$2" > "$SYSFS_GPIO/gpio$1/value" || die "could not write GPIO $1"; }
get() { cat "$SYSFS_GPIO/gpio$1/value" 2>/dev/null || echo "?"; }

# Park: buffers Hi-Z, /PROGRAM released, clock low, data high. This is the
# state the 10K pull-ups on the adapter already hold before anything runs,
# so re-asserting it can never surprise the target.
park() {
    put "$GATE" 1
    put "$PROG" 1
    put "$CCLK" 0
    put "$DIN" 1
}

gate_is_on() { [ "$(get "$GATE")" = "0" ]; }

case "$CMD" in

init)
    check_pins
    export_pin "$DIN" out
    export_pin "$CCLK" out
    export_pin "$PROG" out
    export_pin "$GATE" out
    export_pin "$DONE" in
    park
    echo "exported and parked:"
    echo "  din=gpio$DIN  cclk=gpio$CCLK  prog=gpio$PROG  gate=gpio$GATE  done=gpio$DONE (input)"
    echo
    echo "The buffer gate is HIGH, so all three 74HC125 channels are Hi-Z and"
    echo "nothing of ours reaches the FPGA yet. Bring-up stage 3 says: with the"
    echo "adapter connected and the target powered, NOTHING on the target should"
    echo "have changed. Confirm that before running 'gate on'."
    echo
    echo "DONE currently reads: $(get "$DONE")   (expected 0 with the EPROM pulled)"
    ;;

status)
    printf 'din  gpio%-3s = %s\n' "$DIN" "$(get "$DIN")"
    printf 'cclk gpio%-3s = %s\n' "$CCLK" "$(get "$CCLK")"
    printf 'prog gpio%-3s = %s   (/PROGRAM, active low)\n' "$PROG" "$(get "$PROG")"
    printf 'gate gpio%-3s = %s   (0 = we drive the board, 1 = Hi-Z)\n' "$GATE" "$(get "$GATE")"
    printf 'done gpio%-3s = %s   (input)\n' "$DONE" "$(get "$DONE")"
    ;;

gate)
    [ $# -ge 1 ] || die "gate needs 'on' or 'off'"
    case "$1" in
        on)
            put "$GATE" 0
            echo "gate LOW - the 74HC125s now drive DIN, CCLK and /PROGRAM."
            echo "Stage 3 check: toggle din/cclk/prog with 'set' and confirm the"
            echo "74HC125 outputs follow. If any output fights (hot IC, level"
            echo "stuck between rails), run 'gate off' immediately - something"
            echo "else is still driving those nets. Is the EPROM really out?"
            ;;
        off)
            put "$GATE" 1
            echo "gate HIGH - buffers Hi-Z, the target is on its own again."
            ;;
        *) die "gate takes 'on' or 'off', got '$1'" ;;
    esac
    ;;

set)
    [ $# -ge 2 ] || die "set needs a signal name and a value, e.g. 'set cclk 1'"
    case "$2" in 0|1) ;; *) die "value must be 0 or 1, got '$2'" ;; esac
    case "$1" in
        din)  put "$DIN" "$2" ;;
        cclk) put "$CCLK" "$2" ;;
        prog) put "$PROG" "$2" ;;
        gate) die "use 'gate on|off' - the polarity is inverted and easy to get backwards" ;;
        done) die "done is an input; read it with 'status'" ;;
        *)    die "unknown signal '$1' (din, cclk or prog)" ;;
    esac
    echo "$1 = $2"
    gate_is_on || echo "note: the gate is HIGH, so this did NOT reach the FPGA (that is the point of stage 3)."
    ;;

clock)
    [ $# -ge 1 ] || die "clock needs a cycle count"
    case "$1" in ''|*[!0-9]*) die "cycle count must be a number, got '$1'" ;; esac
    _sec=${2:-0}
    gate_is_on || echo "note: the gate is HIGH - these edges go nowhere."
    _i=0
    while [ "$_i" -lt "$1" ]; do
        put "$CCLK" 1
        [ "$_sec" = "0" ] || sleep "$_sec"
        put "$CCLK" 0
        [ "$_sec" = "0" ] || sleep "$_sec"
        _i=$((_i + 1))
    done
    echo "issued $1 CCLK cycle(s), DIN held at $(get "$DIN"); DONE now reads $(get "$DONE")"
    ;;

pulse-program)
    if ! gate_is_on; then
        die "the gate is HIGH - the pulse would not reach the FPGA. Run 'gate on' first."
    fi
    _before=$(get "$DONE")
    put "$PROG" 0
    sleep 1                 # spec minimum is sub-microsecond; 1s is for the meter
    put "$PROG" 1
    _after=$(get "$DONE")
    echo "/PROGRAM pulsed low for ~1s and released."
    echo "DONE: $_before -> $_after"
    if [ "$_after" = "0" ]; then
        echo "OK - stage 4 passes. The FPGA cleared its configuration and is"
        echo "waiting for CCLK. A real download can be attempted next."
    else
        echo "STOP - DONE should have gone low. Either the pulse is not reaching"
        echo "/PROGRAM (check the gate and the PD2 path) or the DONE readback is"
        echo "not really connected (check the 10K:15K divider off DB25 pin 13)."
        echo "Do not stream a bitstream until this reads 0."
        # Non-zero so a script driving this over ssh cannot mistake the
        # stage-4 failure for a pass.
        exit 1
    fi
    ;;

release)
    park
    for _p in "$DIN" "$CCLK" "$PROG" "$GATE" "$DONE"; do
        echo "$_p" > "$SYSFS_GPIO/unexport" 2>/dev/null
    done
    echo "parked (gate HIGH, /PROGRAM released) and unexported."
    ;;

*)
    die "unknown command '$CMD' (init status gate set clock pulse-program release)"
    ;;
esac
