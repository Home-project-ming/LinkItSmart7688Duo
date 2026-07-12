---
name: mt7688-hardware
description: Use when calling, choosing between, or troubleshooting any linkit7688-hw MCP tool (gpio_read/write/set_direction, i2c_read/write, spi_transfer, pwm_configure/set_enabled/read, uart_transact, i2s_play, atmega_adc_read, atmega_bridge_raw) that controls the LinkIt Smart 7688 Duo board's hardware — covers safety limits, which tool fits a task, and gotchas not visible from a tool's JSON schema alone.
---

# LinkIt Smart 7688 Duo hardware tools

## Overview

This board has **two chips**: the MT7688 (Linux/OpenWrt) and, bridged over
its own dedicated UART, an ATmega32U4 (Arduino firmware). The
`linkit7688-hw` MCP server exposes 13 tools spanning both sides. Each
tool's JSON schema already documents its parameters — this skill covers
what the schema *can't* tell you: which tool to reach for, safety limits,
and traps that only show up once you actually call something.

## Quick reference — which tool for what

| Need | Tool(s) | One-line gotcha |
|---|---|---|
| Digital pin read/write | `gpio_read` / `gpio_write` / `gpio_set_direction` | Rejected pins are the safety net working, not a bug — see below |
| I2C device | `i2c_read` / `i2c_write` | Fixed to `/dev/i2c-0`, no repeated-start (some devices may see stale data across register-select/read) |
| SPI device | `spi_transfer` | Auto-discovers the sole `/dev/spidev*` node — don't hardcode a bus number |
| PWM output | `pwm_configure` / `pwm_set_enabled` / `pwm_read` | Only channels 0 (GPIO18) and 1 (GPIO19) exist on this board |
| Generic serial device | `uart_transact` | Fixed to `/dev/ttyS1` (pins 45/46) — **not** the ATmega bridge, see below |
| Play audio | `i2s_play` | WAV/base64 only, 100KB cap, playback only — recording is a confirmed dead end, not just missing |
| ATmega analog input | `atmega_adc_read` | channel 0-11 = A0-A11, per-call semantics, not continuous streaming — see below |
| Raw ATmega bridge bytes | `atmega_bridge_raw` | Escape hatch, fixed `/dev/ttyS0` @ 57600 baud, not caller-adjustable |

## Safety: the GPIO blocklist

`gpio_read`/`gpio_write`/`gpio_set_direction` reject certain pins outright
(`"gpio N is blocked: <reason>"`). **This is the safety net working as
intended, not a bug to work around.** Every blocked pin is already
claimed by another peripheral this server exposes its own dedicated tool
for:

| Pins | Claimed by |
|---|---|
| 0-3 | I2S (WM8960 codec) |
| 6-10 | SPI bus |
| 12-13 | ATmega bridge UART (`uart0`, `/dev/ttyS0`) |
| 18-19 | PWM channels 0/1 |
| 20-21 | Console |
| 22-29 | SD/extroot |
| 38 | Labeled "ATmega ISP reset" in the blocklist — this label is likely **wrong** (real evidence points to pin 3 for that role; pin 3 is separately already blocked, for I2S reasons, so it's protected in practice even though the blocklist's stated reason for it doesn't mention ISP reset). Don't rely on pin 38's label being accurate. |
| 45-46 | `uart_transact`'s UART (`uart1`) |

If a `gpio_*` call fails with a blocked-pin error, **don't retry with
`--force` or look for a workaround** — pick a different pin, or use the
dedicated tool for whatever peripheral already owns it (e.g. `spi_transfer`
instead of bit-banging SPI pins as GPIO).

## Byte payload convention: `[]int`, not `[]byte`, except audio

Every tool that sends/receives raw bytes (`i2c_read`/`write`, `spi_transfer`,
`uart_transact`, `atmega_bridge_raw`) uses a **JSON array of ints, each
0-255** — never a `[]byte`-typed field (Go's `encoding/json` always
encodes `[]byte` as a base64 *string*, not an array, which breaks this
MCP SDK's schema). The one exception is `i2s_play`, which takes a base64
**string** for its WAV payload — audio payloads are large enough that
base64 is far more token-efficient than a JSON int array would be. Don't
guess a different encoding for either case.

## `uart_transact` vs. the ATmega bridge — two different physical UARTs

This is the single easiest mistake to make on this board: there are
**three** UARTs, and only one of them is `uart_transact`'s.

| | Device | Pins | Baud |
|---|---|---|---|
| `uart_transact` | `/dev/ttyS1` | 45/46 | caller-chosen |
| ATmega bridge (`atmega_adc_read`/`atmega_bridge_raw`) | `/dev/ttyS0` | 12/13 | fixed 57600 |
| Console (not exposed as a tool) | `/dev/ttyS2` | 20/21 | fixed 57600 |

**If you need anything from the ATmega32U4 side, always use the `atmega_*`
tools — `uart_transact` physically cannot reach it.** There is no
parameter on `uart_transact` that redirects it to the bridge; it's a
different device node entirely.

## `atmega_adc_read`: per-call semantics, not continuous streaming

Each call independently subscribes to the requested channel, waits for
one reading, then unsubscribes again — it does **not** leave the channel
continuously streaming between calls. Practical implications:

- **Reads normally resolve in well under 100ms** (the firmware samples
  every 20ms). You don't need to set `timeout_ms` anywhere near its
  10000ms ceiling defensively — 500-1000ms is comfortably generous for
  normal conditions. A very long timeout mostly just delays how quickly
  you find out something is actually wrong.
- **The returned `value` is the ATmega's raw 10-bit ADC reading (0-1023),
  not a voltage.** It is not rescaled or calibrated to anything.
- **A floating (unconnected) analog pin reads low, jittery values across
  repeated calls (e.g. single digits up to a few dozen) — this is normal
  ADC noise on an unterminated input, not a failure signal.** Don't
  interpret jitter as an error; interpret a hard timeout as one.
- If a call times out, the ATmega firmware may simply not be running
  (e.g. the board was power-cycled since the last flash) — check that
  before assuming the Go-side tool is broken.

## `atmega_bridge_raw`: reach for `atmega_adc_read` first

`atmega_bridge_raw` is a low-level escape hatch — send raw bytes, get a
raw reply, structurally identical to `uart_transact` but fixed to the
bridge's device and baud rate (not adjustable, since the firmware's baud
is hardcoded). For a plain "read one channel" task, always prefer
`atmega_adc_read` — it correctly handles the Firmata
enable/wait-for-report/disable sequence for you. Reach for
`atmega_bridge_raw` only when you need to speak a Firmata message the
high-level tool doesn't cover, or are debugging the wire protocol itself.

## `i2s_play`: playback only — recording is a confirmed dead end, not a gap

Do not suggest or attempt building an `i2s_record` tool, and do not try to
route around the missing capability via other tools (e.g. poking the
WM8960 codec's registers through `i2c_write`/`i2c_read`, or bit-banging
I2S pins via GPIO). This board's capture path has a **confirmed
driver-level bug** that hangs the audio device (the hung process becomes
`SIGKILL`-resistant and requires a physical board reboot to clear) — this
was reproduced repeatedly during design, including with the codec's
capture path explicitly enabled and confirmed powered on, and a
buffer/period-size workaround was tried afterward and also failed to
help. Treat any request to attempt recording as something to explain
(and point back to this finding), not something to try "just in case it
works this time."

## After redeploying the MCP server

`build.ps1` + `deploy.ps1` ship a new binary to the board, but the
MCP client session that's already connected keeps talking to the old,
already-spawned process. **New or changed tools won't appear — and
behavior changes won't take effect — until the MCP client session is
reloaded/reconnected.** If a just-added tool isn't showing up, or a fix
doesn't seem to have taken effect, check whether the session was actually
reloaded before assuming the deploy failed.

## Common mistakes

| Mistake | Why it's wrong | Do this instead |
|---|---|---|
| Using `uart_transact` to talk to the ATmega | Wrong physical UART (`ttyS1`, not `ttyS0`) — it cannot reach the bridge at all | Use `atmega_adc_read`/`atmega_bridge_raw` |
| Treating a blocked-GPIO error as a bug | It's the safety net rejecting a pin already claimed by another peripheral | Use the dedicated tool for that peripheral, or pick a different pin |
| Sending a `[]byte`-shaped guess for a raw-bytes field | Every raw-bytes tool except `i2s_play` expects `[]int` (0-255 each) | Use `[]int`; only `i2s_play` takes a base64 string |
| Maxing out `atmega_adc_read`'s `timeout_ms` defensively | Unnecessary — normal reads resolve in well under 100ms | Use 500-1000ms unless you have a specific reason to wait longer |
| Reading `atmega_adc_read` jitter on a floating pin as a failure | Low, noisy values are the expected behavior of an unconnected ADC input | Only treat an actual timeout error as a failure signal |
| Trying to implement or work around `i2s_record` | Confirmed driver-level bug, not a missing feature — reproduces reliably and hangs the board | Explain the limitation; don't attempt it |
| Expecting a fresh deploy's changes to show up immediately | The connected MCP session still runs the old spawned process | Reload/reconnect the MCP client session after `deploy.ps1` |
