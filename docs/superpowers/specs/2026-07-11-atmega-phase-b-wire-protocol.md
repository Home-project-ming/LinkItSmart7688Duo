<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# ATmega bridge Phase B: wire protocol design — design

Status: designed, not yet implemented
Date: 2026-07-11

## Context

Phase A (`docs/superpowers/specs/2026-07-11-atmega-phase-a-toolchain.md`)
proved the toolchain end to end (compile, network flash via ISP-over-GPIO,
visually-confirmed LED blink) but deliberately touched neither `Serial1`
nor any bridge protocol. This spec covers the second of four phases:

- Phase A: toolchain bring-up — **done**.
- **Phase B (this spec):** wire protocol design — framing, message
  subset, versioning, channel numbering.
- Phase C: ATmega-side firmware implementing the protocol.
- Phase D: MT7688-side decoder + new MCP tool(s).

C and D are not designed yet and are out of scope for this document. This
spec produces no code — it is a protocol design only, to be implemented in
Phase C (firmware) and Phase D (MT7688 decoder + MCP tool).

## Scope decision (from brainstorming)

- **v1 capability: ADC read only.** No digital I/O, no PWM, no servo — the
  MT7688 side already has its own GPIO/PWM tools talking directly to its
  own pins; the only thing the ATmega side offers that the MT7688 side
  fundamentally cannot is analog input (MT7688 has no ADC). Keeping v1
  narrowly scoped to that gap avoids building bridge equivalents of tools
  that already exist natively.
- **Framing: Firmata**, chosen explicitly over a custom text-line protocol
  (which was the recommended default) or a custom binary frame. Firmata is
  an existing, standard, MIDI-style byte-stream protocol with a mature
  Arduino-side library (`Firmata.h`), trading a small amount of protocol
  overhead most callers won't hand-decode themselves against getting most
  of the ATmega-side firmware for free rather than hand-rolling a framer/
  parser.

## Message subset used (v1)

Only two of Firmata's many standard message types are needed for
ADC-read-only:

| Message | Direction | Encoding |
|---|---|---|
| `REPORT_ANALOG` | host (MT7688) → device (ATmega) | Byte 0: `0xC0 \| channel` (channel 0-15). Byte 1: `0x00` (disable) or `0x01` (enable) reporting for that channel. |
| `ANALOG_MESSAGE` | device (ATmega) → host (MT7688) | Byte 0: `0xE0 \| channel`. Byte 1: bits 0-6 of the value (LSB 7 bits). Byte 2: bits 7-13 of the value (next 7 bits). Value = `byte1 \| (byte2 << 7)`. |

No other Firmata message type (`SET_PIN_MODE`, `DIGITAL_MESSAGE`,
`REPORT_VERSION`, any `SysEx` command) is used in v1. This keeps the
MT7688-side decoder to exactly two message shapes rather than a general
Firmata parser.

**Resolution note:** an AVR's `analogRead()` returns a 10-bit value
(0-1023) even though Firmata's 2-byte encoding has room for 14 bits
(0-16383) — expect values to top out at 1023, not the wire format's
theoretical maximum. This isn't a bridge-protocol limitation, it's the
ATmega32U4's actual ADC resolution.

## Read semantics: request/response over Firmata's native streaming model

Firmata's analog reporting is natively a **subscribe-and-stream** model,
not request/response: `REPORT_ANALOG(channel, 1)` tells the device to
start pushing `ANALOG_MESSAGE` continuously (StandardFirmata's default
loop samples and reports at a fixed interval, not once per host request).
This doesn't match the "call once, get one value" shape used by every
other tool this server exposes (`gpio_read`, `i2c_read`, `pwm_read`,
`uart_transact`'s read side, etc.).

**Decision: the MT7688 side bridges this gap per-call, not with a
persistent background reader.** For a single ADC read:

1. Send `REPORT_ANALOG(channel, 1)`.
2. Block reading the bridge UART until an `ANALOG_MESSAGE` for that exact
   channel arrives, or a timeout elapses (mirroring `uart_transact`'s
   `read_timeout_ms` pattern — the exact parameter name and default is a
   Phase D tool-surface decision, not fixed here).
3. Send `REPORT_ANALOG(channel, 0)` to stop the stream again.
4. Return the value (or a timeout error).

This was chosen over the alternative — a long-lived background goroutine
that keeps all channels continuously reported and serves reads from an
in-memory cache — because every other tool in this server is stateless
(open the device, do the operation, close it, one request in ⇒ one
response out) and a persistent background reader would be a real
architectural departure from that pattern for a single tool. The
per-call approach costs a small amount of latency (waiting for the next
sample after subscribing, rather than an instant cache hit) in exchange
for staying consistent with the other 11 tools and not introducing
server-lifetime state. If ADC read latency or polling frequency ever
becomes a real problem, revisit this as a targeted change, not a v1
requirement.

## Channel numbering

Confirmed against this board's real, installed
`Arduino15/packages/LinkIt/hardware/avr/0.1.7/variants/smart7688/pins_arduino.h`
— not assumed from a generic Leonardo reference, since this board's
variant file has already been shown (Phase A, `LED_BUILTIN`) to deviate
from generic Leonardo defaults:

- **A0-A5** (channels 0-5): dedicated analog-input-only pins (D18-D23).
- **A6-A11** (channels 6-11): share physical pins with digital D4, D6, D8,
  D9, D10, D12 respectively.

Firmata's channel number is Arduino's own analog pin index (0 = A0, 1 =
A1, ... 11 = A11) — the Arduino core's `analogRead()`/`analogPinToChannel()`
already handles the translation to the raw AVR ADC MUX channel internally
(see `analog_pin_to_channel_PGM` in the variant file); the bridge protocol
and Phase D's MCP tool only need to work in terms of this 0-11 index, not
the raw hardware ADC channel number.

**v1 accepts channel 0-11.** No restriction to just A0-A5: A6-A11 are
usable analog inputs too, and v1 has no digital I/O feature that could
conflict with them. If a future phase adds digital I/O, that phase must
account for A6-A11's dual role — not a Phase B concern.

No collision with pins already claimed elsewhere in this project: the
bridge UART itself is the ATmega's hardware `Serial1` (D0/D1, confirmed in
`pins_arduino.h`'s pin-out comment block), the onboard LED is D13, and the
ISP pins are D14-D17 — none overlap A0-A11's D4/D6/D8/D9/D10/D12/D18-D23.

## Baud rate

**57600**, the Firmata library's own default (`Firmata.begin()` with no
argument). No project-specific reason to deviate has come up; keeping the
default avoids a needless mismatch if a future contributor pulls in
`Firmata.h` example code or documentation that assumes it.

## Explicitly not designed here (deferred to Phase C/D)

- **Exact bridge UART device node on the MT7688 side.** `uart_transact`'s
  existing `/dev/ttyS1` is a *different* physical UART (pins 45/46); the
  ATmega bridge uses pins 12/13, a separate node entirely.
  `CLAUDE.md` already flags this as unverified — confirm via `dmesg`/
  `ls /dev/ttyS*` on real hardware in Phase C or D, don't guess a node
  name now.
- **Whether the standard `Firmata` Arduino library installs and compiles
  cleanly for this board** (`LinkIt:avr:smart7688`, 8MHz/3.3V, manually
  side-loaded core per Phase A's workaround). Not yet attempted. If it
  doesn't install cleanly via `arduino-cli lib install Firmata`, Phase C
  will need to investigate why (version pin, board.txt gap, etc.) rather
  than this spec assuming success.
- **Exact Firmata message bytes for the two messages above, re-verified
  against the actual installed library source** once Phase C installs it.
  The encoding documented above is the standard, published Firmata wire
  format (stable across Firmata versions) and is not expected to differ,
  but per this project's standing discipline of verifying platform
  specifics against real sources before writing code (established after
  the UART termios and ATmega toolchain investigations), Phase C's
  implementer should cross-check against the installed `Firmata.h`/
  `Firmata.cpp` before finalizing the firmware, not merely trust this
  document.
- **The Phase D MCP tool's exact signature** (name, parameter names,
  default timeout value). This spec fixes the wire-level read semantics
  (subscribe → wait-for-report → unsubscribe) but not the Go-level
  function signature built on top of it.
- The pin 3/38 GPIO blocklist discrepancy (carried forward from Phase A,
  still unfixed, still not blocking anything since no Phase C/D code
  touches MT7688 GPIO 3).

## Testing

No automated tests in this phase — it produces a protocol design, not
code. Phase C's firmware and Phase D's Go decoder each get their own
testing treatment when they're built (Phase C likely mirrors Phase A's
"physical verification" approach since ADC readings need a real analog
signal to be meaningful; Phase D's Firmata byte-encoding/decoding logic
is pure and unit-testable the way every prior Go tool's validation logic
has been).

## Open questions for implementation

See "Explicitly not designed here" above — all four items are genuine
unknowns intentionally deferred to Phase C/D investigation rather than
guessed here, following the same discipline Phase A used for the board
manager URL and FQBN.

Resolved during design: v1 scope (ADC-read-only), framing choice
(Firmata over custom text/binary), the two-message subset
(`REPORT_ANALOG`/`ANALOG_MESSAGE`), request/response semantics layered
over Firmata's native streaming model (per-call subscribe/wait/unsubscribe,
not a persistent background cache), channel numbering (0-11, grounded in
this board's real `pins_arduino.h`), and baud rate (57600, library
default).
