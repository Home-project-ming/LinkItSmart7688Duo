<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# MCP server: UART tool — design

Status: approved, not yet implemented
Date: 2026-07-11

## Context

`mt7688-mcp-server` currently exposes 9 tools: GPIO (read/write/direction),
I2C (read/write), SPI (`spi_transfer`), and PWM (`pwm_configure`/
`pwm_set_enabled`/`pwm_read`) — all shipped and live-verified (see
`docs/superpowers/specs/2026-07-10-mcp-server-design.md` and
`docs/superpowers/specs/2026-07-11-spi-pwm-design.md`). That spec's roadmap
table deferred I2S and the ATmega UART bridge to their own future design
cycles, and this session separately confirmed the MT7688 side has **no ADC
hardware at all** (no `/sys/bus/iio/devices`, no ADC pinmux group — ADC only
exists on the ATmega32U4 side per `CLAUDE.md`'s own board table, so it's
blocked on the ATmega bridge work, not independently doable).

User-set order for remaining hardware coverage: **UART (this spec) → I2S →
ATmega UART bridge → ADC** (ADC last because it depends on the bridge work
landing first).

## Live hardware findings, 2026-07-11

Checked directly over SSH before finalizing this design, same discipline as
the SPI/PWM phase:

- **No general-purpose ADC.** `/sys/bus/iio/devices/` doesn't exist. The
  only "adc" hits anywhere on the system are the WM8960 audio codec's
  internal `Left ADC`/`Right ADC` DAPM widgets (`/sys/kernel/debug/asoc/...`)
  — part of the I2S audio codec, not a general-purpose peripheral, and not
  reachable the way GPIO/I2C/SPI/PWM are. Confirms ADC is ATmega-side-only.
- **`uart1` (pins 45, 46) is free and pin-routed.** `dmesg` confirms
  `10000d00.uart1: ttyS1 at MMIO 0x10000d00 (irq = 29, base_baud = 2500000)
  is a 16550A`, and `mt7688_pinmux get` shows `uart1` currently selected to
  `[uart]` (not `gpio`). This is a separate, independent UART from `uart0`
  (pins 12/13, ATmega bridge, already blocked) and `uart2` (pins 20/21,
  console, already blocked) — using it doesn't touch either.
- **`base_baud=2500000`** is a real hardware constant (16550A UART clocked
  for a 2.5 MHz base rate) — used below to bound the `baud_rate` parameter
  to something the hardware can actually represent, rather than leaving it
  unbounded.
- Many other `/dev/ttyS3`-`/dev/ttyS15` nodes exist but none of them appear
  as a group in `mt7688_pinmux get` — they're generic kernel-registered
  UART minor numbers with no pin routing on this board, not usable.

## Goals (v1.2)

- Expose one raw, stateless UART transact primitive: configure baud rate,
  write bytes, read back whatever arrives within a timeout window.
- Extend the GPIO blocklist to cover pins 45/46, same rationale as the
  SPI/PWM pins: once claimed by this tool, a `gpio_write` on them would be
  electrically meaningless rather than erroring.

## Non-goals (v1.2)

- I2S, the ATmega UART bridge, ADC — see Context above; each needs its own
  future design cycle (ADC specifically needs the ATmega bridge to exist
  first, since it has no MT7688-side hardware path at all).
- Configurable data bits / parity / stop bits / flow control. Fixed to 8
  data bits, no parity, 1 stop bit ("8N1"), no flow control — covers the
  overwhelming majority of simple serial devices (debug consoles, AT-command
  modems, simple sensors), and every real call would otherwise repeat the
  same defaults. Only `baud_rate` is exposed, since that's the one thing
  that genuinely varies device-to-device.
- A stateful/session-oriented UART API (separate configure/write/read
  tools with a port kept open across calls). Rejected in favor of a single
  stateless `uart_transact` call (open → configure → write → read-with-
  timeout → close), consistent with this server's existing SPI/I2C
  precedent of not carrying hardware state across MCP tool calls. A
  session-oriented API would suit continuous/unsolicited-traffic listening
  better, but that's not this tool's job — reconsider only if a concrete
  use case needs it.
- Pinmux switching — assumes `uart1`'s current `[uart]` selection (already
  correct, confirmed above) and never reconfigures it.

## Tool surface (v1.2)

| Tool | Signature | Notes |
|---|---|---|
| `uart_transact` | `(baud_rate int, tx_data []int, read_timeout_ms int, max_read_bytes int) -> {rx_data []int}` | Fixed to `/dev/ttyS1` (uart1, pins 45/46) — hardcoded, not discovered, since this board has exactly one free general-purpose UART and it's a stable, kernel-assigned path (unlike SPI's dynamically-numbered bus). `tx_data` may be empty (`[]`) for a read-only/listen call — unlike `spi_transfer`, UART's write and read are not symmetric, so there's no structural reason to require non-empty data. |

`rx_data` is `[]int`, not `[]byte`, applying the same standing rule as
`spi_transfer`/`i2c_read` (Go's `encoding/json` base64-encodes `[]byte`,
breaking the MCP SDK's auto-generated array schema).

## Validation

- `baud_rate`: `1`-`2500000` (upper bound is `uart1`'s real hardware
  `base_baud`, confirmed above — not an arbitrary guess).
- `tx_data`: `0`-`256` bytes (0 is valid, see Tool surface above).
- `read_timeout_ms`: `0`-`25500`. Upper bound comes from the chosen timeout
  mechanism (`VTIME`, a single byte in units of 0.1s — see "Timeout
  handling" below) being unable to represent anything longer; `0` is valid
  and means "return immediately with whatever's already buffered,
  non-blocking."
- `max_read_bytes`: `1`-`4096`.

## Timeout handling

Uses POSIX termios' standard non-canonical-mode read-timeout mechanism:
`Cc[unix.VMIN] = 0`, `Cc[unix.VTIME] = read_timeout_ms / 100` (integer
division, converting to deciseconds — `VTIME` has no finer resolution than
0.1s, so e.g. a requested `read_timeout_ms: 150` rounds down to `VTIME=1`,
i.e. an actual wait of 100ms, not 150ms; `read_timeout_ms` under 100 rounds
down to `VTIME=0`, which is the non-blocking immediate-return case, same as
requesting `0` explicitly). With `VMIN=0`, a `read()` call returns as soon
as at least one byte is available, or after `VTIME` decisecond-units elapse
with nothing received (returning zero bytes, not an error — silence is a
valid, expected outcome for a query nothing answers). This is a standard,
portable technique, not something invented for this board.

## Baud rate configuration — lower risk than SPI, but still verify

Uses `golang.org/x/sys/unix`'s termios support (`IoctlGetTermios`/
`IoctlSetTermios` against `TCGETS`/`TCSETS2`-family requests) rather than
hand-deriving ioctl request numbers the way SPI's `spidev` ioctls required
— `x/sys/unix` already ships correct, per-architecture-generated termios
ioctl constants and a `Termios` struct (this is a much more commonly-used
part of the package than raw `spidev` support, so it's had far more
real-world exercise). The intended mechanism is the modern Linux `BOTHER`
flag plus explicit `Ispeed`/`Ospeed` fields, which supports an arbitrary
integer baud rate directly rather than requiring a lookup table mapping
common rates (9600, 115200, ...) to fixed `B*` constants.

**What's NOT nailed down yet, deliberately:** the exact field names/types on
`unix.Termios` for `Ispeed`/`Ospeed` and whether `x/sys/unix`'s
`IoctlSetTermios` targets `TCSETS` or `TCSETS2` by default on
`linux/mipsle` are not asserted here as fact. Given SPI's hand-derived ioctl
code had two real bugs caught only by careful code review (see the SPI/PWM
spec's "Live verification" section), the implementation task for this
tool must read the actual `golang.org/x/sys/unix` source for this Go
version/platform before writing the real termios-configuration code —
the same discipline Task 3 of the SPI/PWM plan used to verify
`IoctlSetPointerInt` against real source rather than trusting memory. If
`BOTHER`+`Ispeed`/`Ospeed` turns out unavailable or behaves unexpectedly on
this specific kernel (3.18.23), the fallback is a `B*`-constant lookup
table covering common rates (1200 through 921600) — noted here as the
fallback plan, not designed in detail, since the primary approach is
expected to work and detailed fallback design would be wasted if unneeded.

## Safety: GPIO blocklist extension

Add to `blockedPins` in `blocklist.go`:

- `45` — UART1 (`uart1` pinmux group, currently active)
- `46` — UART1 (`uart1` pinmux group, currently active)

Same rationale as the SPI/PWM entries: a `gpio_write` on these pins
wouldn't error today, it would just be electrically meaningless once
`uart_transact` and the kernel's `uart1` driver own them.

## Error handling

Same posture as the rest of the server: validation errors (bad baud rate,
tx_data too long, timeout out of representable range) are rejected before
touching hardware. A read timeout with zero bytes received is **not** an
error — it's a normal, meaningful result (nothing arrived in the window).
Hardware/driver errors (open failure, ioctl failure) are surfaced with
enough detail (errno, the attempted baud rate) to diagnose.

## Source layout

Same three-way platform split as SPI:

- `uart.go` — platform-independent validation (baud_rate, tx_data length,
  read_timeout_ms, max_read_bytes ranges) and the `UARTTransact` wrapper
  that validates then calls the platform-specific `uartTransact`.
- `uart_linux.go` (`//go:build linux`) — real termios configuration +
  write + timed read against `/dev/ttyS1`.
- `uart_other.go` (`//go:build !linux`) — stub, same rationale as
  `spi_other.go`/`i2c_other.go`.
- `uart_test.go` — validation-only unit tests, no hardware needed.

`main.go` gains one new `mcp.AddTool` registration following the existing
handler shape.

## Build & deploy

No changes — same `build.ps1`/`deploy.ps1`.

## Testing

- Validation logic: plain `go test`, no hardware needed.
- Actual UART transfers (including whether the `BOTHER`/`Ispeed`/`Ospeed`
  approach actually works on this kernel): can't be meaningfully tested off
  the hardware. Manual verification against the real board after deploy —
  ideally against a real device on the other end (even a simple USB-serial
  loopback or a device known to echo at a known baud rate), since (unlike
  SPI, where the ioctl succeeding at all was a meaningful signal even with
  nothing attached) a UART transact call succeeding with `rx_data: []`
  proves less on its own — it's also exactly what "wrong baud rate, nothing
  understood us" looks like.

## Open questions for implementation

- Confirm `x/sys/unix`'s exact `Termios`/`IoctlSetTermios` API shape for
  `linux/mipsle` before writing real code (see "Baud rate configuration"
  above) — this is the one piece of real technical risk in this design,
  deliberately left unresolved here rather than guessed at.
- Whether a real device is available to test against for live verification,
  or whether verification is limited to "the ioctl succeeds and doesn't
  error" the way `i2c_write` was left partially unverified in the original
  MCP server phase.
