<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# ATmega bridge Phase A: toolchain bring-up — design

Status: approved, not yet implemented
Date: 2026-07-11

## Context

The ATmega UART bridge is the last major item on this project's hardware
roadmap (GPIO/I2C/SPI/PWM/UART/I2S are shipped — see
`docs/superpowers/specs/2026-07-11-spi-pwm-design.md`'s roadmap table).
Unlike every prior tool, this one requires programming a **second,
completely separate processor** (the ATmega32U4) that this project has
never touched — no toolchain installed, no FQBN confirmed, no sketch ever
compiled or flashed. `CLAUDE.md`'s own ATmega section is still mostly
TODOs.

Given the scope (new toolchain + wire protocol + AVR firmware + MT7688-side
decoder + MCP tools), this was decomposed into four phases during
brainstorming:

- **Phase A (this spec):** toolchain bring-up — prove `arduino-cli` can
  compile and flash *something* onto this board's ATmega, with no bridge
  protocol or MCP server changes involved at all.
- **Phase B:** wire protocol design (framing, versioning, message types).
- **Phase C:** ATmega-side firmware implementing the protocol.
- **Phase D:** MT7688-side decoder + new MCP tool(s).

B/C/D are not designed yet and are out of scope for this document.

## Live findings, 2026-07-11

- **`arduino-cli` is not installed** on the Windows dev machine (confirmed:
  `arduino-cli version` → command not found).
- **No USB connection is available for this bring-up.** The Windows PC and
  the board are not physically co-located right now. USB remains available
  as a manual fallback later (the user can walk over and plug in directly
  if network-based flashing doesn't pan out), but Phase A's primary path
  must not depend on it.
- **The board's stock firmware already ships a complete ISP-over-GPIO
  flashing mechanism** — this was the major discovery of this phase's
  investigation, and it resolves what looked like the hardest open
  question (how does network/OTA upload even work on this board, since
  `CLAUDE.md` left it as a TODO):
  - `/usr/bin/avrdude` (v6.1) and `/etc/avrdude.conf` are pre-installed
    (opkg package `avrdude`).
  - `/usr/bin/run-avrdude` is a pre-installed helper script:
    ```sh
    #!/bin/sh
    mt7688_pinmux set spi_s gpio
    avrdude -c linuxgpio -C /etc/avrdude.conf -p m32u4 -e -v -Uflash:w:$1 $2
    ```
    It takes a `.hex` file path as `$1` (and passes any `$2` through to
    avrdude as extra arguments), temporarily repurposes the `spi_s` pinmux
    group (GPIO pins 14-17) to plain GPIO, and uses avrdude's `linuxgpio`
    programmer to bit-bang the standard 6-pin AVR ISP protocol directly
    against the ATmega32U4 — **no bootloader, no serial upload protocol,
    no dependency on the shared UART bridge (pins 12/13) at all.**
  - The exact ISP pin mapping, read directly from the live, in-use
    `/etc/avrdude.conf` entry (not inferred): `reset = 3, sck = 15, mosi =
    16, miso = 17`. sck/mosi/miso match the known `spis` pinmux group
    exactly (pins 15-17 of the 14-17 group), giving high confidence in
    this reading.
  - `/etc/init.d/yunbridge` / `/sbin/yunbridge` is a **different, unrelated
    feature** — a login-shell-over-the-bridge-UART service (currently
    disabled, `option disabled '1'` in `/etc/config/yunbridge`), not a
    network upload relay. Initially assumed to be the upload mechanism;
    investigation showed it's not. Not needed for Phase A.
- **Important side-finding, flagged but explicitly NOT fixed in this
  spec:** the ISP `reset` pin is GPIO **3**, not GPIO 38. The existing
  `blockedPins` entry for pin 38 ("ATmega ISP reset (held by the core)")
  was based on an indirect inference (the `wdt` pinmux group name), not a
  directly-verified source, and now looks likely to be mislabeled — pin 3
  is currently *not* blocked and was actively toggled during this
  project's original GPIO live-testing (the "RST" LED the user saw light
  up red was almost certainly this exact reset line, not a generic status
  LED). This is a real correctness question about already-shipped safety
  code, but reconciling/fixing `blocklist.go` is out of scope for this
  spec — it doesn't block Phase A (nothing here calls the MCP server's
  `gpio_write` on pin 3), and there's no real ATmega firmware yet for a
  stray reset to actually disrupt. Tracked for a future fix, not deferred
  indefinitely.

## Goals (Phase A)

- Install and configure `arduino-cli` on the Windows dev machine for this
  board.
- Confirm the exact FQBN for "LinkIt Smart 7688 Duo".
- Compile a minimal sketch (blink the ATmega's own onboard LED — nothing
  MT7688-side, nothing bridge-related) to a `.hex` file.
- Flash it to the real board over the network, using the board's own
  stock `run-avrdude` mechanism via SSH — no USB required.
- Get a real, physically-observable confirmation (the user watching the
  LED actually blink) that the whole path — compile, transfer, ISP flash
  — works end to end on this exact hardware.

## Non-goals (Phase A)

- Any wire protocol, bridge framing, or MT7688↔ATmega communication
  logic. The test sketch only touches the ATmega's own onboard LED; it
  never reads or writes `Serial1`.
- Any change to `mt7688-mcp-server`. This phase doesn't add, modify, or
  plan any MCP tool.
- Fixing the pin 3/38 blocklist discrepancy (see "Live findings" above) —
  tracked separately.
- Getting `arduino-cli`'s own built-in network/OTA upload feature (if the
  LinkIt board core even supports one) working. The manual
  compile-locally → scp `.hex` → SSH `run-avrdude` path is what's actually
  verified to exist on this board; if `arduino-cli` turns out to have
  native support for this board's upload mechanism, that's a nice-to-have
  simplification to try later, not a Phase A requirement.

## Approach

1. Install `arduino-cli` (Windows binary/installer).
2. Add the board manager URL needed to pull the MediaTek/Seeed "LinkIt
   Smart 7688 Duo" board definition (exact URL to be found during
   implementation — `CLAUDE.md` says Arduino Boards Manager already knows
   how to find it, implying it's either a well-known third-party board
   index or bundled; confirm rather than guess).
3. `arduino-cli board listall | grep -i 7688` to get the exact FQBN.
4. Write a minimal sketch under a new top-level directory reserved for
   ATmega-side code (fills in `CLAUDE.md`'s "Code lives in (TODO: ...)"
   line for the ATmega side — exact directory name decided during
   implementation, following this repo's own convention of not
   documenting a path until code actually lives there):
   ```cpp
   void setup() {
     pinMode(LED_BUILTIN, OUTPUT);
   }
   void loop() {
     digitalWrite(LED_BUILTIN, HIGH);
     delay(500);
     digitalWrite(LED_BUILTIN, LOW);
     delay(500);
   }
   ```
5. `arduino-cli compile --fqbn <fqbn> --output-dir <dir> <sketch-dir>` to
   produce a `.hex`.
6. `scp -O` the `.hex` to the board (reusing the exact same SSH key +
   4-legacy-flag mechanism already used by `deploy.ps1` throughout this
   project — no new credential handling).
7. `ssh` in and run `run-avrdude /path/to/sketch.hex`.
8. Ask the user to visually confirm the LED is blinking.

## Safety

`run-avrdude` temporarily repurposes GPIO pins 14-17 (`spi_s` group) and
toggles pin 3 (reset) during the flash. Neither conflicts with anything
`mt7688-mcp-server` currently exposes: no shipped tool uses the `spi_s`
group (the shipped `spi_transfer` tool uses the separate, dedicated main
SPI bus at pins 7-10), and pin 3, while not currently blocklisted, isn't
claimed by any tool either — it was only ever touched manually during a
one-off live GPIO demo early in this project. No MCP server changes are
needed or made in this phase specifically because of this — the risk is
noted for completeness, not because it requires a code change here.

## Testing

There is no automated test for this phase — it's infrastructure bring-up
on a second physical processor, verified by direct observation (does the
LED blink). This mirrors how the original SD-extroot/Entware-Python
provisioning work earlier in this project was verified (real commands run
against real hardware, documented narratively) rather than the
spec→plan→TDD→subagent-review pipeline used for MCP server tool work —
Phase A produces no Go code and doesn't touch `mt7688-mcp-server` at all.

## Open questions for implementation

- Exact board manager URL and FQBN — not yet looked up, first thing to
  confirm once `arduino-cli` is installed.
- Exact new top-level directory name for ATmega-side code — decide during
  implementation and update `CLAUDE.md`'s TODO line once it exists,
  per this repo's existing documentation convention.
- Whether `run-avrdude`'s `$2` argument (extra avrdude args) is ever
  needed for this simple case — likely not, `$1` alone should suffice for
  a plain `-Uflash:w:` flash.

Resolved during design: the whole "how does network upload even work"
question (ISP-over-GPIO via the board's pre-installed `run-avrdude`, not
a serial bootloader or the `yunbridge` service), verification strategy
(visual LED confirmation, no USB/serial needed), and scope (toolchain
proof only, no protocol/firmware/MCP work yet).
