<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# XCS20 slave-serial download via the ATmega bridge — implementation

Status: **working on real hardware, design running.** 2026-08-09: `top.bit`
downloaded in 4,888 ms with DONE asserted (within 2% of the 4.8s predicted
here), and `blinkio0.bit` then ran a counter off the on-chip oscillator,
blinking an LED at 0.479 Hz — a 1998 part executing logic synthesised in
2026. Three bugs had to be fixed first; all are in "Gotchas" below, and each
one had passed every check that was supposed to catch it.
Date: 2026-08-08 (verified 2026-08-09)

Supersedes §5 ("7688-side transport choice") of
`2026-07-30-xcs20-slave-serial-download-design.md`. Everything else in
that document — the target board's DB25 wiring, the bitstream format, the
slave-serial protocol, the risk register — still stands and is not
repeated here.

## Why the transport changed

The original design put the bit-bang loop on the MT7688: five sysfs GPIOs
(four out, one in), ~3 writes per bit, 2–5 seconds. It listed candidate
pins "4, 5, 11, 14–17, 30–37, 39–44" as an open item to confirm against
the vendor pin-out.

Confirming it killed the design. The Developer's Guide's Table 8 (p. 25)
lists what the **Duo** actually breaks out: **P1, P8 and P9** — and P8/P9
are the console UART. One usable pin against the five the adapter needs.
The candidate list was for the bare 7688 module's SoC pins, not for this
board's header.

The ATmega32U4 on the same board has 24 free I/O and already talks to the
7688 over a 57600-baud bridge. So it drives the adapter, and the 7688
only ships bytes.

## Architecture

```
build.ps1 → scp .bit → 7688 tmpfs
          → MCP fpga_configure
             → ConfigureViaATmega  (fpga_bridge.go)
                → serialStream on /dev/ttyS0, one fd for the whole download
                   → Firmata SysEx 0x7E
                      → fpga-download.ino
                         → D8–D12 → 74LS125 → DB25 → XCS20
```

**The whole slave-serial sequence lives in the firmware.** This is the
load-bearing decision. `ConfigureSlaveSerial` in `fpga.go` drives one pin
per call, three calls per bit — over a bridge that is 534,432 round
trips. Moving the sequence to the side that owns the pins also makes DONE
honest: the firmware reads the real pin instead of the host guessing at a
value it cannot see.

`sysfsPort` and `ConfigureSlaveSerial` are left in `fpga.go`, tested and
unused. They are correct for a board with free GPIOs; this is not one.

## Wire protocol (SysEx command 0x7E)

| Sub | Direction | Body | Firmware action |
|---|---|---|---|
| `0x01` BEGIN | host → | — | park, take bus, /PROGRAM pulse, verify DONE fell, 100ms clear wait |
| `0x02` DATA | host → | `<n> <encoded…>` | decode, clock out MSB-first, add `n` to its counter |
| `0x03` END | host → | — | 16 startup clocks, read DONE, park |
| `0x04` ABORT | host → | — | park; no reply |

BEGIN and END reply `7E <sub> <status> <done> <n0..n3>`; DATA and ABORT
do not reply.

**Chunk = 49 raw bytes.** Firmata's `MAX_DATA_BYTES` is 64 (read from the
installed `FirmataConstants.h`), and a DATA message spends 3 bytes on
command/subcommand/length. 49 raw bytes encode to exactly 56 (49×8 = 392
= 56×7, no padding), which fits with room to spare.

**DATA is not acknowledged.** A 61-byte chunk takes ~10.6ms to arrive at
57600 baud. Clocking it out takes ~0.5ms, but only once the unpacking is
written with shifts — the first version used `bit/7` and `bit%7` per bit,
which on a divider-less 8MHz AVR cost ~8ms and left the firmware barely
keeping up. See gotcha 12. With that fixed there is roughly 10x of headroom
and a per-chunk round trip would only add latency. Loss is
caught at the end instead: the firmware counts payload bytes and END
reports the total, which the host compares against what it sent. A short
count is reported as lost chunks, **not** as `done=false` — otherwise a
bridge problem would send someone probing the adapter for a fault that
is not there.

**Measured cost:** 455 chunks, 27,725 bytes on the wire, **4.81s** at
57600 baud (asserted by `TestChunkCountForRealBitstreamIsWhatTheDocsClaim`).

## 7-bit encoding

SysEx data bytes must have bit 7 clear. Firmata's own helper sends one
7-bit byte per half-byte, doubling the transfer. This packs densely
instead — a plain MSB-first bit stream, 7 bits per wire byte — for 8/7
expansion instead of 2×. Over 22,268 bytes that is 4.7s instead of 7.7s.

The DATA length field carries the **raw** byte count, which is what tells
the firmware how many bits of the final encoded byte are padding. That is
also what makes a ragged tail chunk work (see gotcha 3).

## Pin assignment

Frozen in firmware, because the hardware is soldered:

| ATmega | 74LS125 | DB25 | Net |
|---|---|---|---|
| D8 | 1A→1Y | 2 | PD0 / DIN |
| D9 | 2A→2Y | 3 | PD1 / CCLK |
| D10 | 3A→3Y | 4 | PD2 / /PROGRAM |
| D11 | 4A→4Y | 5 | PD3 / buffer gate (LOW = we drive) |
| D12 | ← 10K:15K divider | 13 | SLCT / DONE |

The firmware caches port registers at setup rather than hardcoding a
pin-to-port map, because this board's variant has already been shown to
deviate from a stock Leonardo (Phase A's missing `LED_BUILTIN`).

`docs/duo_breakout_pinout.jpeg` is a photo of the Seeed breakout's
silkscreen and shows both halves of this at once: the ATmega row is
labelled `D8/PB4 D9/PB5 D10/PB6 D11/PB7 D12/PD6`, confirming the mapping
above, and the MT7688 row is `PORST JTDO MDI_* USB_* TXD2 RXD2 GND 3V3`
— **not one general-purpose GPIO**, which is the whole reason this
transport exists.

**Power-up order replaces the GATE pull-up.** While the ATmega is in
reset its pins are inputs, so D11 floats and the 74LS125's channel-4
output is indeterminate — if it settles low it enables the target's
buffers and drives garbage at the FPGA's config pins.

That needs two things at once, though: the LS125 powered (its VCC comes
from the target's 5V) *and* the ATmega in reset. **Boot the 7688 fully,
then power the target board**, and the window never opens — with the
target off the LS125 drives nothing, and DB25.5 is held high by the
target's own 1K pull-up (RP1/RP2). This is the order §3 of the 2026-07-30
design already recommended. A 10K from D11 to 3.3V would close the window
independently; it was considered and skipped in favour of the sequencing.

**The residual case: do not flash the ATmega while the target is
powered.** avrdude resets the chip, which is exactly the dangerous
combination, and bring-up means flashing repeatedly. Power the FPGA board
down first.

Consequence if it does happen: nothing is damaged. /PROGRAM sits low and
the FPGA stays unconfigured, so the green LED never lights and the board
looks broken. Power-cycling in the right order clears it.

## Gotchas

Recorded because each one cost real time today.

1. **`go build` does not compile tests.** Deleting `openSysfsPort` built
   clean and broke five tests. `go vet ./...` or `go test ./...` is the
   check that matters.

2. **`slaveSerialPort` is the wrong seam for a remote transport.** The
   first attempt implemented it over the bridge with a buffering adapter
   underneath. Everything downstream of that choice was wrong; the fix
   was to abandon the interface, not to patch the adapter.

3. **A fixed-size buffer silently drops the tail.** 22,268 is not a
   multiple of 49 — the last 22 bytes never fill the buffer and never
   flush. The FPGA gets 22,246 bytes and DONE never rises, with nothing
   in the logs to say why. The length field in DATA is what prevents
   this; `TestConfigureViaATmegaSendsEveryByteIncludingShortTail` is what
   keeps it prevented.

4. **SysEx needs its F0/F7 frame.** Writing `7E 01 …` raw to the bridge
   does nothing at all: bytes below 0x80 with no command context are
   discarded by Firmata's parser.

5. **`sysexCallbackFunction` is `(byte command, byte argc, byte *argv)`.**
   Declaring `argc` as `int` fails to compile with three near-match
   candidates and a wall of conversion errors that does not obviously say
   "your second parameter is the wrong width".

6. **`sendSysex` takes three arguments** — `(command, bytec, bytev)`, not
   `(bytec, bytev)`.

7. **Counts above 127 cannot travel as bytes.** The received-byte count
   is 22,268; it goes out as four 7-bit groups. A reader that treats them
   as plain bytes gets a plausible wrong answer rather than an error.

8. **The ATmega has its own USB power port** (silkscreened "USB Power &
   MCU USB Port"), separate from the 7688's. Unpowered, `avrdude` reports
   "AVR device not responding" — carried over from Phase A, still the
   first thing to check.

9. **`/tmp` on the board is tmpfs and gets cleared.** `scp` the `.hex`
   immediately before flashing, with no troubleshooting in between.

10. **Read `mcp-server` before executing it, or it faults.** Launch it as
    `cat /usr/local/bin/mcp-server >/dev/null 2>&1; exec /usr/local/bin/mcp-server`
    — both `.mcp.json` files and `build.ps1` already do.

    Demand-paging this 7MB binary off the SD-backed overlayfs
    (`/dev/mmcblk0p1`, ext4 under overlayfs, kernel 3.18) hands the CPU
    garbage. Reading the whole file through the ordinary read path first
    populates the page cache correctly; `exec` then maps good pages.

    Reproduced on demand, 2026-08-09:

    | page cache | 4 consecutive runs |
    |---|---|
    | warm | `0 0 0 0` |
    | dropped, exec straight away | `139 132 132 2` (SIGSEGV, SIGILL ×2) |
    | dropped, `cat` first | `0 0 0 0` |

    This supersedes an earlier note here that said to wait a minute or two
    after boot. Waiting appears to work only because something else reads
    the file in the meantime; uptime is not the variable, and a cold cache
    on a long-uptime board fails exactly the same way. Nothing is wrong
    with the binary — both the July and August builds behave identically.

    It matters most for a download: a fault partway through 4.8s of
    streaming would look like a wiring problem, not a paging one.

11. **One firmware at a time.** `adc-bridge.ino` and `fpga-download.ino`
    both own Serial1; flashing one displaces the other, so
    `atmega_adc_read` stops working while download firmware is loaded.
    Merging them is listed below as future work.

12. **The AVR has no divider, and `%7` per bit ate the timing budget.**
    The first `doData` unpacked with `enc[bit/7] >> (6 - bit%7)`. Software
    division at 392 bits per chunk is ~8ms on this 8MHz part, against the
    10.6ms a chunk takes to arrive — a few percent of headroom, not the
    25x an earlier draft of this document claimed. The 64-byte UART buffer
    overflowed, SysEx framing desynced, and every later chunk was dropped.

    The symptom was clean and worth recognising: END timed out, and
    probing the firmware afterwards reported **11,172 bytes received =
    228 × 49**, a halt at almost exactly half of the 455 chunks. A
    partial, round multiple of the chunk size means the firmware stopped
    parsing — not that the adapter is miswired.

    Fixed with a shift accumulator (~0.06ms). Equivalence to the old
    unpacking was checked for every length 1–49 before spending a flash
    cycle.

13. **Firmata's `sendSysex` splits every byte into two 7-bit bytes.** The
    reply parser read the fields flat, so each one landed a position
    early: a `NOT_STREAMING`/`done=false` reply decoded as **status OK
    with DONE asserted**. A false success on the one question this tool
    exists to answer.

    Its unit tests passed throughout, because the fixtures were
    hand-built with the same wrong assumption. Fixture and parser now
    share `encodeReplyFields`, and
    `TestParseReplyOnCapturedFirmwareBytes` pins the format to bytes
    actually captured off the board.

    General lesson, and the reason both of these survived to bring-up: a
    test written from the same belief as the code under test proves the
    belief is self-consistent, not that it is true. Anything describing a
    wire format needs one fixture taken from the wire.

14. **Board I/O numbers are not package pin numbers, and nothing will tell
    you.** This cost more than the rest of the session combined.

    UCF `LOC` takes **package pins** — `P1`–`P208` on a PQ208. The FPT-2's
    connector is labelled with **I/O indices**, and the two do not line up:
    the board's "I/O 0" is package pin **P2**. (P1 is GND; the user I/O in
    package order run P2, P3, P4, P5, P8, P9, … — 156 of them, which is why
    a board label above 160 cannot be an I/O index at all.)

    Given `LOC = "P180"` the tools do exactly as told: P180 is a real user
    I/O, PAR routes it, DRC passes, bitgen writes a bitstream, the download
    succeeds and DONE rises. The design simply drives a pin nobody is
    watching. There is no error to notice.

    What made it expensive was not the mistake but what was built on top of
    it. "The LED does not blink" became "the counter is frozen", then "GSR
    is held", then "OSC4 does not oscillate" — four rounds of bisection, a
    supporting theory for every reading, and eventually a plan to solder in
    an external clock. **All of it downstream of an indicator wired to a pin
    the design never touched.** OSC4 had been working the whole time.

    The step that would have collapsed it on the first try: **hold the pin
    at a hard constant and confirm the indicator responds, before
    interpreting anything dynamic.** `build/io0low.v` in the FPGA_XCS20 repo
    does exactly that and is worth keeping for the next unknown pin.

    A meter made it worse, not better. Readings of 0.15V, 0.67V, "about 2V",
    0V, 1.5V, 5V were all taken from a pin that was floating or from a
    damaged LED, and each one got a theory attached to it. A DMM averages
    over ~100ms; against a signal of unknown frequency and duty it returns a
    number that cannot be interpreted. The LED — blinks or does not — was
    the better instrument as soon as it was wired to the right pin.

15. **OSC4 works; no external clock is needed.** The 2026-07-30 design said
    so and was right, though its evidence (`Number of OSC: 1 out of 1` in
    the MAP report) never justified the claim — that line proves the tool
    allocated the resource, not that the silicon oscillates. Now confirmed
    the only way it can be: a counter clocked from F490 blinking an LED at
    the predicted rate.

    Drive it through a **`BUFGS`**. `verilog2edif.py` inserts a global
    buffer for a clock arriving on a top-level *port*, but OSC4 is a
    blackbox instance and its output is just an internal net, so nothing
    recognises it as a clock. Whether the fabric would carry it without one
    was never isolated — the pin error masked every result either way — but
    the dedicated clock network is where an oscillator output belongs, and
    `blinkio0.v` uses it.

## Testing

`go test ./...` — all green, no hardware. The bridge tests cover the
encoding round trip at **every** chunk length 1–49 (not just the tidy
one), SysEx framing including partial frames and the ATmega's
boot-time `REPORT_VERSION` noise, the split-count decode, the ragged
tail, lost-chunk detection, BEGIN refusal, and bus release on every exit
path.

Firmware compiles for `LinkIt:avr:smart7688`: 9,528 bytes flash (33%),
653 bytes RAM.

Verified end to end as far as the hardware allows:

```
.\build.ps1 -Design top -NoBuild -Download -DryRun
  design : top_par.ncd
  part   : s20pq208
  bits   : 178144
  dry run: parsed and validated, nothing was driven
```

That exercises build.ps1 → scp → ssh → MCP server → bitstream parse →
structured reply. **Everything past the dry run is untested.** No byte
has ever reached the FPGA.

## Bring-up, in order

1. Flash the firmware, **with the FPGA board powered down**. ATmega USB
   power connected (gotcha 8), and copy the `.hex` immediately before
   flashing (gotcha 9).
   ```
   arduino-cli compile --fqbn LinkIt:avr:smart7688 atmega-firmware/fpga-download
   ```
2. Bench-check the adapter alone, no target: 3.3V in → 5V out on all four
   channels; divider 5V in → 3.0V out.
3. Let the 7688 boot fully (gotcha 10), **then** power the target board,
   **EPROM removed**, adapter connected. Green LED off. This order is what
   keeps the buffers Hi-Z through the ATmega's reset.
4. Dry run (above). Proves the software chain without driving a pin.
5. Real download: `.\build.ps1 -Design top -Download`. Expect
   `done=true`, green LED on, ~5s.
6. If DONE never rises: check the byte count in the error first. A short
   count is a bridge problem; a full count with `done=false` is bit
   order, stream integrity, or wiring — scope CCLK and DIN at the
   74LS125 outputs.

## Future work

1. **Merge the two firmwares** so ADC and download coexist — the
   SysEx command and the ADC reporting do not actually conflict.
2. **Raise the baud rate.** Table 8 puts the ATmega UART Lite at 0.5 Mbps;
   500000 would cut 4.8s to ~0.6s. Needs `atmegaBridgeBaud` and
   `Serial1.begin()` changed together.
3. **Per-chunk retry** if the wire turns out to be lossier than expected.
   The byte counter already detects loss; it just cannot repair it.

## Files

- `atmega-firmware/fpga-download/fpga-download.ino` — new
- `mt7688-mcp-server/fpga_bridge.go` — protocol, encoding, sequencer
- `mt7688-mcp-server/fpga_bridge_linux.go` / `_other.go` — transport
- `mt7688-mcp-server/fpga_bridge_test.go` — new
- `mt7688-mcp-server/fpga.go` — `ConfigureFPGA` lost its pins parameter
- `mt7688-mcp-server/main.go` — tool schema and description
- `FPGA_XCS20/build.ps1` — `-Pins` deprecated, board IP default refreshed
