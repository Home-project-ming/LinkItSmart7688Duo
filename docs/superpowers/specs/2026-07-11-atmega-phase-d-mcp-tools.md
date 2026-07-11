<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# ATmega bridge Phase D: MT7688-side decoder + MCP tools — design

Status: designed, not yet implemented
Date: 2026-07-11

## Context

The last of four planned phases for the ATmega UART bridge:

- Phase A: toolchain bring-up — **done**.
- Phase B: wire protocol design (`docs/superpowers/specs/2026-07-11-atmega-phase-b-wire-protocol.md`) — **done**.
- Phase C: ATmega-side firmware (`docs/superpowers/plans/2026-07-11-atmega-phase-c-adc-firmware.md`) — **done, live-verified**. Confirmed live: bridge UART is `/dev/ttyS0` on the MT7688 side, firmware correctly starts/stops a well-formed `ANALOG_MESSAGE` stream in response to `REPORT_ANALOG`.
- **Phase D (this spec):** MT7688-side Go decoder + new MCP tool(s), the final piece connecting the bridge to `mt7688-mcp-server`'s existing 11 tools.

## Scope decision (from brainstorming)

Two tools, not one:

- **`atmega_adc_read`** — the high-level, purpose-built tool matching this server's existing style (`gpio_read`, `i2c_read`, `pwm_read`: call once, get one value).
- **`atmega_bridge_raw`** — a low-level escape hatch that sends arbitrary bytes over the bridge and reads back a raw response, chosen explicitly over staying at just the one ADC-specific tool. This intentionally exceeds Phase B's "v1 is ADC-read-only" framing at the *capability* level (the wire protocol itself is unchanged — this tool doesn't add new message types, it just lets a caller speak whatever the ATmega firmware already understands, including future extensions without another MCP server round-trip to add a dedicated tool each time).

## Tool surface

| Tool | Signature | Notes |
|---|---|---|
| `atmega_adc_read` | `(channel int, timeout_ms int) -> {value int}` | `channel`: 0-11 (A0-A11, per Phase B/C). Sends `REPORT_ANALOG(channel, enable)`, loop-reads and parses the bridge stream until an `ANALOG_MESSAGE` for that exact channel arrives or `timeout_ms` elapses, sends `REPORT_ANALOG(channel, disable)` before returning (best-effort even on timeout — see "Cleanup on timeout" below), returns the 10-bit value (0-1023 in practice, see Phase B's resolution note). |
| `atmega_bridge_raw` | `(tx_data []int, timeout_ms int, max_read_bytes int) -> {rx_data []int}` | Structurally identical to the existing `uart_transact` (write once, single timed read) but **fixed** to `/dev/ttyS0` at **57600 baud** — neither is caller-adjustable, since the ATmega firmware's baud rate is hardcoded and letting a caller pick a mismatched one would just desync communication with no benefit. `[]int` payload encoding matches every other binary-payload tool in this server (`i2c_read`/`write`, `spi_transfer`, `uart_transact`) — not base64, since these payloads are small (Firmata command bytes, not audio). |

Both tools accept/return byte values as `[]int` (one JSON array element per byte) or plain `int`, following this server's established rule that `[]byte` breaks the MCP SDK's schema (established in the SPI/PWM design doc).

## Firmata encode/decode design

Pure, portable logic in `firmata.go` — no I/O, fully unit-testable with plain `go test`, no hardware needed (mirrors `i2s.go`'s WAV-validation split and `pwm.go`'s `pwmWritePeriodFirst` pure-function pattern):

- `encodeReportAnalog(channel byte, enable bool) []byte` — returns the 2-byte `REPORT_ANALOG` command (`0xC0|channel`, `0x00` or `0x01`).
- A message-parsing function that, given an accumulating byte buffer, extracts zero or more **complete** messages from the front and returns the leftover unconsumed suffix (a message can legitimately arrive split across two separate `read()` syscalls — see "Read loop" below). Recognizes exactly two shapes, both 3 bytes total:
  - `ANALOG_MESSAGE` (`0xE0`-`0xEF` + 2 data bytes): channel = low nibble of the first byte, value = `byte1 | (byte2 << 7)`.
  - `REPORT_VERSION` (`0xF9` + 2 data bytes): recognized and **discarded**, not surfaced to the caller — this is the unsolicited boot-time message Phase C's code-quality review found (`Firmata.begin(Serial1)` sends one automatically). Any well-formed message the parser doesn't need is silently dropped, not treated as an error; only genuinely malformed/unrecognized leading bytes trigger the resync behavior below.
  - Any other leading byte (not `0xE0`-`0xEF`, not `0xF9`): treated as desynced/stray data, dropped one byte at a time, parsing retried from the next byte. This is a defensive fallback, not expected to trigger in normal operation (the two message shapes above should be the only bytes this bridge ever sees given the firmware's fixed behavior), but keeps a single stray or misaligned byte from wedging the read loop.

## Read loop and timeout semantics (`atmega_adc_read`)

`uart_transact`'s single `unix.Read()` call (VMIN=0/VTIME=`readTimeoutMs/100`) isn't sufficient here: a `REPORT_ANALOG` reply can take more than one `read()` call to fully arrive, and an unrelated message (the boot-time `REPORT_VERSION`, or in principle a stale message from a previous call) can arrive first and needs to be skipped rather than misread as the answer. `atmega_adc_read` therefore:

1. Opens `/dev/ttyS0`, configures termios (raw mode, 57600 baud, VMIN=0/VTIME= a short fixed per-call sub-timeout, not the caller's full `timeout_ms` — see below).
2. Writes `encodeReportAnalog(channel, true)`.
3. Loops: `unix.Read()` (bounded by the short VTIME sub-timeout) → append to an accumulating buffer → run the parser → if an `ANALOG_MESSAGE` for the requested `channel` is found, stop and keep that value. Otherwise (buffer drained of complete messages, or a `REPORT_VERSION`/stray byte was discarded) continue looping, checking overall elapsed wall-clock time against the caller's `timeout_ms` each iteration.
4. Writes `encodeReportAnalog(channel, false)` (best-effort — see below).
5. Closes the fd, returns the value or a timeout error.

The per-`read()` VTIME sub-timeout is a small fixed value (on the order of 100-200ms), independent of the caller's `timeout_ms` — it just paces the loop so overall elapsed time can be checked reasonably often; it is not itself a user-facing parameter.

**Cleanup on timeout:** if step 3 never finds a match before `timeout_ms` elapses, step 4 (disable) still runs before returning the timeout error, on a best-effort basis (its own error, if any, is not what's returned to the caller — the timeout is the real failure being reported). This matters because leaving a channel's reporting enabled after a caller gives up would otherwise pollute a later, unrelated call's read loop with stray `ANALOG_MESSAGE` traffic for a channel nobody is asking about anymore.

## Concurrency: a single bridge mutex

Both tools operate on the same physical link (`/dev/ttyS0`), and `REPORT_ANALOG` is a **stateful** enable/disable protocol — unlike this server's register-style tools (`gpio_read`, `i2c_read`, etc.), where each call is independently atomic against the hardware, two concurrent bridge operations could genuinely corrupt each other (e.g., two concurrent `atmega_adc_read` calls for different channels racing to read the same shared byte stream, each risking misattributing or losing the other's reply). A single `sync.Mutex` in `atmega_linux.go`, held for the full duration of each bridge transaction (open through close), serializes all access regardless of which tool initiated it. This is a new pattern for this server — every prior tool's device access has been safely concurrent-callable because the underlying operations were independently atomic at the hardware/kernel level; this is the first genuinely stateful, multi-step wire protocol this server drives, so it's the first tool needing this.

(Note: `uart_transact`, on the unrelated `/dev/ttyS1` link, has no equivalent lock — it's a single write+read against a stateless-from-the-caller's-perspective device, and is out of scope for this spec regardless.)

## Validation

- `atmega_adc_read`: `channel` must be 0-11 (matching the firmware's confirmed `ANALOG_CHANNEL_COUNT`). `timeout_ms` bounded to a reasonable range, e.g. 1-10000 (ADC reads should resolve within tens of milliseconds given the firmware's 20ms sampling interval; no reason to allow much longer blocking than `uart_transact`'s existing bounds suggest is reasonable for this server).
- `atmega_bridge_raw`: mirrors `uart_transact`'s existing bounds exactly, since it's the same read-once shape — `tx_data` length 0-256, `timeout_ms` 0-25500 (this one *is* a single VTIME-bounded `unix.Read()`, so it inherits VTIME's uint8-deciseconds representable range same as `uart_transact`), `max_read_bytes` 1-4096.

## File layout

- `firmata.go` — pure encode/decode (see above). New file, no platform split needed (pure Go, no syscalls).
- `atmega.go` — parameter validation + `AtmegaADCRead`/`AtmegaBridgeRaw` wrapper functions, mirroring `uart.go`'s shape.
- `atmega_linux.go` — real termios/syscall work, the shared mutex, the read loop. Reuses the same raw-`unix.Open`-based approach as `uart_linux.go` (bypassing Go's runtime poller, for the same VMIN/VTIME-interaction reason already established there) — a small internal helper for "open + configure termios at a fixed device/baud" is extracted and shared between `atmega_adc_read`'s loop and `atmega_bridge_raw`'s single-shot transaction, avoiding duplicating the termios dance a third time in this codebase.
- `atmega_other.go` — non-linux stub, matching `i2c_other.go`/`spi_other.go`/`uart_other.go`.
- `firmata_test.go` — unit tests for the pure encode/decode logic (message parsing across split reads, `REPORT_VERSION` discarding, resync-on-stray-byte, multi-message-in-one-buffer).
- `atmega_test.go` — validation-only unit tests (channel/timeout bounds), same style as every prior tool's validation tests.

`main.go` gains two new `mcp.AddTool` registrations.

## Error handling

- Validation errors (out-of-range `channel`, `timeout_ms`, `tx_data` length, `max_read_bytes`): rejected before any device access, matching every prior tool.
- `atmega_adc_read` timeout (no matching `ANALOG_MESSAGE` within `timeout_ms`): a clear timeout error, distinct from a generic I/O failure — the caller likely wants to know "the ATmega didn't answer in time" vs. "the serial port itself failed to open."
- Device open/termios/write/read failures: surfaced with the underlying syscall error wrapped in context (device path, operation), matching `uart_linux.go`'s existing error-wrapping style.

## Safety: GPIO blocklist

No changes needed. Pins 12/13 (the physical UART0 lines this bridge uses) are already blocklisted (`"ATmega bridge UART"`, added during Phase A/earlier work) — this spec only adds `/dev/ttyS0` character-device access, not raw GPIO manipulation, so no new blocklist entries apply.

## Testing

- `firmata.go`'s parser and encoder: plain `go test`, no hardware needed — this is the bulk of the genuinely interesting logic (split-read reassembly, `REPORT_VERSION` filtering, resync behavior) and the most valuable to cover with unit tests.
- `atmega.go`'s validation: plain `go test`, same style as every prior tool.
- Actual bridge communication (both tools' real device I/O, the mutex under concurrent calls): can't be meaningfully tested off the hardware. Manual verification against the real board after deploy — following Phase C's precedent, a floating A0 pin (or a deterministic 3.3V/GND jumper, if the user is willing this time) is sufficient to confirm `atmega_adc_read` round-trips a real value; `atmega_bridge_raw` can be verified by sending the same raw `REPORT_ANALOG` bytes Phase C's manual test already used and confirming the tool decodes what Phase C decoded by hand.

## Open questions for implementation

- The short per-`read()` VTIME sub-timeout used inside `atmega_adc_read`'s loop (proposed 100-200ms) is a reasonable starting value, not derived from a hard constraint — fine to adjust during implementation if testing suggests a different value paces better, as long as it stays well under any reasonable `timeout_ms` the tool's own validation would accept.
- Whether concurrent-call behavior (two `atmega_adc_read`/`atmega_bridge_raw` calls at once, actually exercising the mutex) gets a dedicated live test, or is just trusted from the code review — likely decided pragmatically during implementation once it's clear how easy that is to trigger deliberately against the real board.

Resolved during design: two-tool scope (`atmega_adc_read` + `atmega_bridge_raw`, the latter an explicit, deliberate choice to exceed Phase B's ADC-only wire-protocol framing at the tool-capability level), the shared bridge mutex (a new pattern for this server, justified by this being the first stateful multi-step wire protocol it drives), the split between pure Firmata encode/decode (`firmata.go`, heavily unit-tested) and platform I/O (`atmega_linux.go`, manually verified), and fixing `atmega_bridge_raw`'s baud rate (unlike `uart_transact`'s caller-adjustable one) since the ATmega firmware's baud isn't negotiable.
