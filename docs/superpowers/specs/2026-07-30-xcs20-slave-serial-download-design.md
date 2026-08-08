<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# XCS20 slave-serial download from the LinkIt Smart 7688 Duo — design

**Status: design / pre-implementation.** Goal: configure the Xilinx
XCS20-4-PQ208 (original Spartan, 1998) on a year-2000 development board
directly from the 7688 Duo, replacing the PC parallel port the board was
designed for. The bitstream comes from the already-working ISE 4.2i
toolchain at `d:\LEAP_FPT_2_Xilinx_XCS20\build` (`.\build.ps1 -Design <n>
-Synth`) — the working copy of the [FPGA_XCS20
repo](https://github.com/MingChihLung/FPGA_XCS20); the local directory
name and the repo name differ.

Every claim below is labeled: **[schematic]** read off the sheets in
`docs/*.jpeg`, **[verified]** proven by running code in this session,
**[empirical]** reported from past use of the real board, **[memory]**
datasheet values from model knowledge (safe because we use ≥1000×
margins), **[open]** must be measured/obtained before or during bring-up.

## 1. What the target board provides

Sheets available: `printer_port_pin_out.jpeg` (sheet 1, POWER&PRINTER/FPT-2),
`xcs20_download_pin.jpeg` (sheet 3, DOWNLOAD/XCS20),
`cpld_download_to_xcs20.jpeg` (sheet 4, EPROM/XCS20),
`bypass.jpeg` (sheet 5, BYPASS/XCS20 — **only decoupling caps CB1–CB20,
0.1µF/50V; no isolation jumpers**). Sheet 2 (FPGA itself) is missing **[open]**.

- Logic rail VCC = **5V** (DC jack → LM7805). **[schematic]**
- DB25 "printer port" (CON9) with **1K pull-ups to 5V** (RP1/RP2) and
  **180pF to GND** (C1–C17) on every line. **[schematic]**
- Download path (sheet 3): two 74HC125 tri-state buffers at 5V gate the
  port onto the FPGA's config pins:

  | DB25 pin | Net | Role | Path |
  |---|---|---|---|
  | 2 | PD0 | **DIN** | PD0 →300Ω→ U4C →100Ω→ DIN |
  | 3 | PD1 | **CCLK** | PD1 →300Ω→ U5A →100Ω→ CCLK |
  | 4 | PD2 | **/PROGRAM** | PD2 →300Ω→ U4D →100Ω→ /PROGRAM |
  | 5 | PD3 | **buffer gate**: LOW = port drives DIN/CCLK//PROGRAM; HIGH = all three Hi-Z | PD3 →300Ω→ /OE of U4C, U4D, U5A |
  | 6 | PD4 | optional **DONE hold-low** (U4B: input grounded, PD4 = its /OE). Keep HIGH = never used | |
  | 13 | SLCT | **DONE readback**: DONE (5.1K pull-up R3) → U4A (always enabled) → SLCT | |
  | 15 | /PR_ERROR | board-presence detect (VCC via 1K/diode/100Ω) — optional "target is powered" sense | |
  | 18–25 | GND | common ground (also the connector-orientation test points) | |

  All **[schematic]**.
- Boot path (sheet 4): XC9536 CPLD + AM27C020 EPROM self-configure the FPGA
  at power-on; green LED D1 indicates success. CPLD JTAG unconnected.
  **[schematic]**
- **The EPROM is socketed. With the EPROM removed, downloading through the
  DB25 works and lights the green LED — proven on the real board in the
  past.** This resolves the CPLD-contention question (no contention with
  EPROM out), proves the FPGA is strapped for slave serial, and gives a
  known-good success indicator. **[empirical]**

Standard operating procedure therefore: **pull the EPROM, download over
DB25, green LED = success.**

## 2. Bitstream facts (all [verified] this session)

- `.bit` = self-describing header + payload. Header: 2-byte-BE-length
  fields; tags `a` design, `b` part (`s20pq208` — check it!), `c` date,
  `d` time; tag `e` + **4-byte BE payload length**. Header length VARIES
  (71 bytes for top.bit, 73 for test2.bit) — never hardcode offset;
  reference parser: `bitparse.py` in the
  [FPGA_XCS20 repo](https://github.com/MingChihLung/FPGA_XCS20).
- Payload = 22,268 bytes = 178,144 bits = exactly the XCS20 configuration
  length. **Shift each byte MSB-first into DIN** — byte-for-byte equal to
  bitgen's `.rbt` ASCII bit stream (bit-for-bit compare, full match).
- Stream self-frames: 8×'1' fill, `0010` preamble, 24-bit length count
  (0x02B7D9), then data frames. Extra leading '1's and extra trailing
  CCLKs are harmless [memory].
- For the EPROM alternative route: `promgen -w -p mcs -u 0 <design>.bit`
  output = payload with **each byte bit-reversed** (LSB-first serial-PROM
  convention), 22,268 bytes exact match.

## 3. Electrical interface (the only hardware-risk part)

**Direct 3.3V-GPIO-to-DB25 connection is forbidden.** Two independent
reasons: (1) each line pulls to 5V through 1K — a 7688 pin configured as
input would sink ~(5−3.6)/1K ≈ **1.4mA continuously through its ESD clamp**
into the 3.3V rail (out of spec, cumulative damage risk) [computed];
(2) 74HC125 at 5V has VIH(min) = **3.5V** — a 3.3V high is not even a
guaranteed logic high, so it might not work anyway [memory].

**Chosen design: 74AHCT125 (or 74HCT125) powered from the target's 5V**
as the 3.3V→5V driver, plus a resistor divider for the one 5V→3.3V input:

```
7688 GPIO_A ──→ AHCT 1A →1Y ──→ DB25.2  (PD0 / DIN)
7688 GPIO_B ──→ AHCT 2A →2Y ──→ DB25.3  (PD1 / CCLK)
7688 GPIO_C ──→ AHCT 3A →3Y ──→ DB25.4  (PD2 / /PROGRAM)
7688 GPIO_D ──→ AHCT 4A →4Y ──→ DB25.5  (PD3 / buffer gate)
DB25.13 (SLCT/DONE) ──10K──┬──→ 7688 GPIO_E (input)
                           15K
                           GND        (5V × 15/25 = 3.0V high) [computed]
DB25.6 (PD4): leave unconnected — its 1K pull-up keeps it HIGH (inactive). [schematic]
DB25.18..25 ──→ common GND with the 7688 (mandatory)
AHCT VCC ──→ target 5V. Tap: EPROM socket pin 32 (VCC, socket is empty!)
             or the LM7805 output tab. AHCT GND → same common ground.
All four AHCT inputs: 10K pull-up to the 7688's 3V3 pin, and tie the
four /OE (1/4/10/13) to GND (always enabled).
```

- AHCT VIH = 2.0V → 3.3V drive is solidly high. AHCT output = full 5V
  CMOS swing → beats the HC125's 3.5V threshold. [memory]
- Pull-ups make the idle/boot state safe: PD3 HIGH (buffers Hi-Z),
  /PROGRAM HIGH, before any software runs. [computed]
- RC check: AHCT output (~25Ω) into 180pF ‖ 1K-to-5V → τ ≈ 5ns; the
  on-board 300Ω→HC125 (~5pF) adds ~2ns. Safe to well beyond 1MHz CCLK;
  our bit-bang will run at ~10–100kHz. [computed]
- Power sequencing: with target ON and 7688 OFF, the dead 3.3V rail lets
  the AHCT inputs sag low → buffers enabled, /PROGRAM driven low → FPGA
  merely held unconfigured, nothing is damaged; it configures normally on
  the next run. Preferred order anyway: power both, then run the tool.
  [computed]
- BOM: 1× 74AHCT125N (DIP-14), 5× 10K, 1× 15K, DB25 connector (gender:
  **verify on the physical board [open]** — identify pin 1 by continuity:
  pins 18–25 all read 0Ω to board GND), hookup wire, breadboard.

## 4. Slave-serial protocol (the software contract)

Timing uses margins so wide that datasheet precision is irrelevant
(slave-serial CCLK has **no minimum frequency** — the logic is fully
static [memory]; max ≈ 8MHz, we stay ≥50× below).

1. Init pins: PD3=1 (buffers off), /PROGRAM=1, CCLK=0, DIN=1. Verify
   DONE readback (should be 0 if unconfigured, 1 if already configured).
2. PD3=0 — take the bus.
3. **/PROGRAM pulse**: drive 0, hold ≥10ms (spec minimum ~300ns [memory]),
   back to 1. DONE must read 0 now — if not, wiring is wrong: abort.
4. **Wait 100ms** for internal configuration-memory clear (spec is ~ms
   [memory]; INIT is not wired out on this board, so use fixed delay).
5. Stream all 22,268 payload bytes, MSB of each byte first:
   for each bit: DIN=bit → CCLK=1 → CCLK=0. (DIN sampled on rising edge
   [memory].) No gaps needed, arbitrary pauses allowed — but note §5's
   rule that the buffers must stay enabled and CCLK glitch-free for the
   whole stream.
6. **Startup**: hold DIN=1, issue 16 extra CCLK cycles, polling DONE.
   DONE should rise within the first few. [memory]
7. PD3=1 (release the bus), leave /PROGRAM=1. Report DONE state, bytes
   streamed, elapsed time.

DONE never rising ⇒ bit order / stream integrity / length-count problem —
re-check §2 facts first, then probe CCLK/DIN activity at the 74HC125
outputs.

## 5. 7688-side transport choice

**Chosen: GPIO bit-bang from a dedicated Go loop (not the generic MCP
gpio_write, not hardware SPI).**

- Generic `gpio_write` MCP calls: 3 calls/bit × 178k bits over SSH —
  absurd (days). Only for single-stepping during bring-up.
- Hardware SPI (`/dev/spidev32766.1`, mode 0, MSB-first — matches §2
  perfectly): tempting, but the MT7688's SPI bus is shared with the boot
  NOR flash **which holds the root filesystem**, so kernel flash traffic
  can inject CCLK edges between our transfers. Every stray edge shifts a
  garbage bit — the FPGA has no framing recovery. Chunked spi_transfer
  (256B/call over SSH) leaves thousands of such windows. Rejected unless
  bit-bang proves too slow (mitigation would be: single uninterrupted
  kernel-side stream, everything in tmpfs, and accepting residual risk).
- **Bit-bang in Go with pre-opened sysfs `value` fds** (kernel 3.18 → no
  fast gpiochip uAPI; the existing `gpio.go` open/close-per-write pattern
  must NOT be reused for streaming — keep fds open, `pwrite` "1"/"0").
  ~3 writes/bit × ~2–5µs ≈ **2–5s total download**. Fast enough.

Pin assignment **[open — verify before wiring]**: needs 5 free GPIOs
(4 out + 1 in). Non-blocklisted candidates: 4, 5 (i2c group), 11,
14–17 (spi_s group — shared with the ATmega ISP lines; free when no
`run-avrdude` is in progress), 30–37, 39–44. Which of these reach the
Duo's physical header must be confirmed against the vendor pinout
(Developer's Guide Table 16 / pinout card) **and** empirically: export
the pin, toggle it, watch with a DMM before connecting anything.
Note: using 14–17 while ATmega tools are active is fine (they are the
ATmega's SPI pins, its firmware ignores them), but a concurrent firmware
flash would clash — don't flash the ATmega during a download.

## 6. Software deliverables

1. **`fpga_configure` MCP tool** (mt7688-mcp-server, existing style):
   - Input: `{bit_path string, pins {din,cclk,prog,gate,done int},
     dry_run bool}` — the `.bit` is `scp`-ed to board tmpfs first (22KB;
     avoids base64-in-JSON size games; precedent: file-path inputs are new
     but i2s_play's 100KB cap shows why inline payloads don't scale).
   - Parses the header per §2 (Go port of `bitparse.py`), **rejects a
     part-name ≠ `s20pq208`** unless `force`, streams per §4–5, returns
     `{done bool, bits int, ms int, part string, design string}`.
   - Pin numbers are inputs (with defaults) because the wiring is not yet
     frozen; validation reuses `CheckBlocked`.
   - Unit tests (no hardware): header parse (both real .bit files as
     testdata + truncated/corrupt), sequencing against a fake pin
     interface (asserts /PROGRAM pulse before first CCLK, MSB-first order,
     PD3 released on every error path — `defer`-based cleanup).
2. **Bring-up shell script** (`provisioning/06-xcs20/01_manual_probe.sh`):
   busybox-only, exports pins, single-steps /PROGRAM and a few CCLKs so a
   human with a DMM can verify each stage of §7 before any Go code runs.
3. **Host glue**: `build.ps1 -Download [-BoardIp ...]` in the XCS20 repo —
   scp the .bit + invoke the tool over SSH (same legacy-algo ssh options
   as `provisioning/README.md`).

## 7. Bring-up procedure (stage → expect → abort-if)

0. **Board offline note:** the 7688 was unreachable at design time
   (`192.168.31.156` timed out); re-verify IP via `ping -4 mylinkit.local`.
1. Bench-check the adapter alone (no target): 3.3V toggles in → 5V
   toggles out on all four channels; divider: 5V in → 3.0V out.
2. Target board, EPROM **removed**, power on, nothing connected: green
   LED should NOT light (no config source) — confirms LED = DONE proxy.
   Measure DB25.13 vs GND: ~0V (DONE low). DB25 pins 18–25: 0Ω to GND
   (this also pins down the connector numbering/mirroring).
3. Connect adapter, 7688 booted, all outputs idle (PD3=1): nothing
   changes. Toggle PD3 0/1 with `gpio_write`: DIN/CCLK//PROGRAM at the
   74HC125 outputs (or FPGA pins) follow PD0/1/2 only when PD3=0.
   Abort-if: any 74HC125 output fights (hot IC, wrong levels) — recheck
   §1's assumption that nothing else drives these nets with EPROM out.
4. Manual /PROGRAM pulse (script from §6.2): DONE readback goes/stays 0.
5. Full download via the MCP tool with `top.bit` (counter) or `blink.bit`.
   Expect: DONE=1, **green LED on**, in ~2–5s.
6. Design-proof: `blink.bit` is built for this — it uses the on-chip
   OSC4 oscillator (no external clock needed, sheet-2 ignorance is fine):
   P27 = ~1.9Hz square (DMM reads ≈ VCC/2), P29 = high, P159 = low.
   [verified through the full toolchain; OSC 1/1 mapped]
   ⚠ P27/P29/P159 were auto-placed; before probing, confirm those QFP
   pins aren't tied to CPLD-driven nets (sheet 2 missing) — probe with
   a 1K series resistor first, or skip and trust the green LED.
7. Regression: power-cycle with EPROM back in → original boot behaviour
   unchanged (nothing on the board was modified — this whole design is
   reversible by unplugging the DB25).

## 8. Risk register (ranked)

| # | Risk | Odds | Impact | Cheap early test | Mitigation |
|---|---|---|---|---|---|
| 1 | 3.3/5V mistake fries 7688 pin or ESD-degrades it | med if rushed | lose 7688 GPIO | stage 1 bench check | AHCT design §3; never direct-connect |
| 2 | DB25 numbering mirrored (device-side view) | med | nothing works, confusion | GND continuity on 18–25 | stage 2 |
| 3 | Wrong 7688 pin not actually on header / blocklisted later | med | rewire | DMM wiggle test §5 | pins are tool inputs, not constants |
| 4 | 26-year-old 74HC125/board dead | low-med | route dead | stage 3 follows PD0→outputs | fall back to EPROM route |
| 5 | Stray CCLK edge mid-stream (loose wire, SPI temptation) | low | download fails, retry | DONE never rises | bit-bang, one continuous stream, short leads |
| 6 | CPLD drives config nets even with EPROM out | ~0 (empirically disproven) | contention | stage 3 abort-if | PD3=1 instantly |
| 7 | `.bit` for wrong part | ~0 (tool-checked) | config fails | header part check | `force` gate |

Genuinely irreversible things on this path: none — no board modification,
EPROM goes back in its socket, CPLD/EPROM contents untouched.

## 9. Alternatives (kept in reserve)

- **Burn a replacement AM27C020** (route around the DB25 entirely): use
  `promgen` output (byte-bit-reversed payload, §2) — but needs an EPROM
  programmer + UV eraser. Good fallback if the 74HC125 path is dead.
- **ATmega32U4 as download engine**: it's 3.3V here, so it solves nothing
  electrically over the 7688, is slower (57600-baud bridge), and adds a
  firmware project. Rejected.
- **PC with real LPT + ISE impact.exe**: needs a legacy PC/PCIe-LPT and
  the vendor's original download program semantics; our DB25 is *not* a
  Xilinx cable, so impact doesn't even speak this board's PD3-gate
  convention. Rejected.
- If the old **vendor download program** for this board still exists, it
  is a golden reference for PD-line semantics — worth grabbing if found.
  **[open]**

## 10. Open items

1. Freeze the 7688 pin choice (Table 16 + physical header + wiggle test).
2. Sheet 2 photo — nice-to-have: FPGA pin↔net map (unlocks safe probing
   of blink's P27/P29/P159 and any real I/O use later).
3. Locate the old PC-side download program (golden reference), if it
   still exists.
4. Board back online → rerun the read-only inspection (`/proc/mtd`,
   `/dev/spidev*`, free-GPIO sanity) that timed out this session.
