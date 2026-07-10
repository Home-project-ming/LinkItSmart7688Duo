<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# MCP server: SPI + PWM tools — design

Status: approved, not yet implemented
Date: 2026-07-11

## Context

`mt7688-mcp-server` (spec: `docs/superpowers/specs/2026-07-10-mcp-server-design.md`)
currently exposes GPIO (read/write/direction) and I2C (read/write) as MCP
tools, live-verified on the real board. Its own v1 spec explicitly deferred
SPI, PWM, I2S, and the ATmega UART bridge.

This document scopes and designs the next increment: **SPI and PWM only**.
It also records the full four-subsystem roadmap (see below) so this repo's
state is self-contained for anyone — including a future session on a
different machine — picking the work back up without re-deriving context.

## Roadmap: the full hardware-coverage picture

Four subsystems were identified as remaining hardware surface. They are not
equally scoped, so they are not being designed or built together:

| Subsystem | Status | Why this grouping |
|---|---|---|
| GPIO, I2C | **Shipped**, live-verified 2026-07-10 | v1, done |
| **SPI, PWM** | **This spec**, v1.1 | Same shape as GPIO/I2C: MT7688-side only, register/bus-level primitives, no new processor or protocol involved. Natural extension of the existing tool pattern. |
| I2S | Surveyed only (pinmux group known: pins 0-3, functions `i2s`/`gpio`/`pcm`); nothing else investigated | Different *shape* of problem, not just a new peripheral: I2S is a continuous audio stream, not a register read/write. What a meaningful MCP tool call even looks like (play a file? stream raw PCM? for how long?) is an open design question in its own right, not a hardware-access question. Needs its own brainstorm cycle before a spec is possible. |
| ATmega UART bridge | Pins 12/13 reserved (blocked in GPIO blocklist); no wire protocol defined; ATmega/Arduino toolchain not yet set up (FQBN still a TODO in `CLAUDE.md`) | Categorically larger: requires writing new AVR-side firmware, defining a wire protocol, and implementing the MT7688-side decoder for it — a second processor's entire toolchain, not an addition to this Go server. Needs its own brainstorm cycle, likely its own multi-phase plan. |

I2S and the ATmega bridge are explicitly **not** designed further in this
document. They stay as entries in this table (and in `CLAUDE.md`'s existing
TODOs) until each gets its own brainstorming session.

## Live hardware findings, 2026-07-11

Checked directly over SSH before finalizing this design (per this project's
own rule: don't invent commands or assume device state, confirm against the
real board first) — same discipline that caught the GPIO 12/13 blocklist gap
in the previous spec.

**SPI:**
- `/dev/spidev32766.1` exists. Bus number `32766` is a MediaTek/kernel
  implementation detail (dynamic bus allocation, not the conventional
  `spidev0.0`); **only chip-select 1 is registered** — no
  `/dev/spidev32766.0` exists, so CS0 has no bound spidev instance in this
  firmware's device tree even though the electrical pin likely exists.
- Pinmux: the main SPI bus (pins 7-10) has **no GPIO-alternative group** in
  `mt7688_pinmux get` at all — it's a fixed-function pin set, not
  switchable. `spi_cs1` (pin 6) is currently pinmux'd to `spi_cs1` (not
  `gpio`).
- `lsmod` shows `regmap_spi` only; `spidev` support is compiled directly
  into this kernel (no separate loadable module), consistent with the
  device node already existing without any `insmod` step.

**PWM:**
- `/sys/class/pwm/pwmchip0` exists (`10005000.pwm`), `npwm=4`. Clean state:
  no channel has ever been exported (`export`/`unexport` present,
  `pwmchip0/pwm0`/`pwm1` subdirs absent).
- Pinmux: `pwm0` (pin 18) and `pwm1` (pin 19) are both currently selected to
  `[pwm]` (not `gpio`) — already correctly muxed, no action needed from this
  server to enable them.
- Channels 2 and 3 (of the 4 the chip reports) have **no known pin mapping**
  — nothing in the pinmux dump routes them anywhere verifiable. `uart2`
  lists `pwm` as an alternate function, but `uart2` is the console (already
  blocked) and out of scope to repurpose.

## Goals (v1.1)

- Expose one raw, stateless SPI transfer primitive.
- Expose PWM configure/enable/read for the two verified-routed channels.
- Extend the existing GPIO blocklist to cover pins now claimed by SPI/PWM,
  for the same reason 12/13 were added for the ATmega bridge: writing to a
  GPIO-muxed-elsewhere pin doesn't error, it just silently does nothing (or
  contends with the real controller) — the existing "safe by construction,
  not just documented" principle.

## Non-goals (v1.1)

- I2S, ATmega UART bridge — see Roadmap above.
- PWM channels 2 and 3 — no verified pin mapping; tool rejects them with a
  clear error rather than guessing, same posture the user already applied
  to `i2c_write` against an unknown device address in the previous phase.
- Multiple simultaneous SPI devices / bus-cs selection — only one spidev
  node exists on this board today. If a second one ever appears, add
  explicit bus/cs selection then (YAGNI now).
- SPI/PWM pinmux switching. This design assumes the pinmux state found
  above (already correct for both) and never reconfigures it. If a pin's
  function needs to change, that's a separate, deliberate action outside
  this server's scope.

## Tool surface (v1.1)

| Tool | Signature | Notes |
|---|---|---|
| `spi_transfer` | `(mode int 0-3, speed_hz int, bits_per_word int, tx_data []int) -> {rx_data []int, device string}` | Full-duplex, stateless — SPI transactions are inherently simultaneous read/write, so one tool covers read-only (caller sends dummy `0x00` bytes) and write-only (caller ignores `rx_data`) alike. Auto-discovers the device via glob on `/dev/spidev*`; errors clearly if zero or more than one match (see "Device discovery" below). `device` is echoed back in the output so the caller can see exactly what was used. |
| `pwm_configure` | `(channel int 0\|1, period_ns int, duty_ns int)` | Exports the channel if not already exported (idempotent under `EBUSY`, same pattern as `exportGPIO`). Handles the period/duty write-order constraint internally (see "PWM write ordering" below). Rejects `channel` outside `{0,1}` with an explicit "not verified routed to a known pin" error. |
| `pwm_set_enabled` | `(channel int 0\|1, enabled bool)` | Same channel restriction. |
| `pwm_read` | `(channel int 0\|1) -> {period_ns, duty_ns, enabled bool, polarity string}` | Reads back all four sysfs attributes. |

`tx_data`/`rx_data` are `[]int`, not `[]byte` — applying the lesson from the
`i2c_read`/`i2c_write` bug in the previous phase (Go's `encoding/json`
always base64-encodes `[]byte`, which the MCP SDK's schema generator
doesn't account for). This is now a standing rule for this codebase, not a
one-off fix.

### Device discovery (SPI)

`spi_transfer` globs `/dev/spidev*` at call time (not cached at startup, in
case of hot-swap or a future device tree change):
- Exactly one match → use it.
- Zero matches → error: no SPI device node found.
- More than one match → error naming all matches, asking the caller to be
  more specific (at which point this tool needs a bus/cs parameter — not
  needed today, since only one node exists).

This avoids requiring the caller to know or guess `32766`, a kernel-internal
bus number with no meaning to an LLM caller and no guarantee of stability
across reboots or firmware updates.

### PWM write ordering

The Linux sysfs PWM ABI requires `duty_cycle <= period` at all times;
writing a `duty_cycle` file with the pin's *current* period unable to hold
the new value fails. `pwm_configure` must therefore, when increasing
duty relative to the currently-stored period, write `period` before
`duty_cycle`; when decreasing period below the currently-stored duty, write
`duty_cycle` before `period`. Read the current values first, then pick the
write order — mechanical, but a real footgun if skipped (verify against the
real board during implementation; this is a known general sysfs-PWM
behavior, not yet reproduced on this specific board).

## Safety: GPIO blocklist extension

Add to the existing `blockedPins` in `blocklist.go`, same style as the
existing entries:

- `6` — SPI CS1 (`spi_cs1` pinmux group, currently active)
- `7`, `8`, `9`, `10` — SPI main bus (fixed-function, no GPIO alternative
  exists per pinmux dump, but blocked defensively rather than relying on
  undefined behavior if a `gpio_export` is attempted anyway)
- `18`, `19` — PWM channels 0/1 (`pwm0`/`pwm1` pinmux groups, currently
  active)

Rationale recorded inline in the blocklist (matching the existing
12/13 comment style): these pins are claimed by SPI/PWM tools as of this
change; a `gpio_write` on them would not error, it would just be
electrically meaningless or contend with the real controller.

## Error handling

Same posture as GPIO/I2C: validation errors (bad channel, bad mode, data
out of range, tx_data too long) are rejected before touching hardware, with
messages specific enough to fix the call. Hardware-layer errors (ioctl
failure, sysfs write failure, ENODEV) are surfaced as-is with enough detail
(errno/path) to diagnose, not swallowed or generalized.

## Known risk: MIPS ioctl encoding for SPI

Flagged during the original hardware survey and still unresolved: the
`_IOC` macro's bit layout (`SIZEBITS`, `_IOC_WRITE` bit position,
`DIRSHIFT`) differs on MIPS from the x86/ARM layout most internet-sourced
`spidev` ioctl constants assume (`SPI_IOC_RD_MODE`, `SPI_IOC_MESSAGE(N)`,
etc.). Using wrong constants doesn't necessarily fail loudly — it can
silently target the wrong ioctl or corrupt the request. I2C's ioctl
(`I2C_SLAVE = 0x0703`) was safe because it's a flat constant unaffected by
this; SPI's aren't.

Mitigation for implementation: derive the request numbers from
`golang.org/x/sys/unix`'s MIPS-generated constants (per-arch files, not
hand-copied from a generic Linux header) wherever it already defines them;
for anything it doesn't (`SPI_IOC_MESSAGE(N)` is parameterized by the
transfer count `N` and unlikely to be predefined), compute it using MIPS's
own `_IOC` parameters rather than the generic/x86 ones, and verify the
computed value against this board's actual kernel headers (available via
the OpenWrt SDK toolchain, or by reading `/usr/include/linux/spi/spidev.h`
if present on-device) before trusting it. Treat this as the highest-risk
part of the SPI implementation — verify with a real transfer against a
known-behavior loopback (MOSI physically tied to MISO, if convenient) or a
known device before considering it correct, the same way the GPIO/I2C phase
treated live-hardware verification as the real gate, not `go test`.

## Source layout

Follows the existing I2C platform-split pattern:

- `spi.go` — platform-independent validation (mode 0-3, bits_per_word 1-32,
  tx_data length 1-256, speed_hz > 0).
- `spi_linux.go` (`//go:build linux`) — device discovery, ioctl-based
  transfer.
- `spi_other.go` (`//go:build !linux`) — stub, so the module still builds
  natively on the Windows dev machine.
- `spi_test.go` — validation-only unit tests (mirrors `i2c_test.go`).

PWM follows the plain-file-I/O `gpio.go` pattern instead (no ioctl, no
platform split needed — sysfs reads/writes compile everywhere, they just
fail at runtime on a non-Linux path, same as `gpio.go` today):

- `pwm.go`, `pwm_test.go`.

`main.go` gains four new `mcp.AddTool` registrations following the existing
handler shape exactly.

## Build & deploy

No changes — same `build.ps1` / `deploy.ps1` used for GPIO/I2C.

## Testing

- Validation logic (channel range, mode range, data length/range): plain
  `go test`, no hardware needed — same split as GPIO/I2C.
- Actual SPI transfers and PWM output: can't be meaningfully tested off the
  hardware. Manual verification against the real board after deploy,
  including the MIPS ioctl risk above specifically.

## Open questions for implementation

- Exact max `speed_hz` for this SPI controller — pick a conservative
  default (e.g. 1 MHz) and confirm/adjust against real hardware; not
  discoverable from pinmux alone.
- Whether `pwm_configure`'s ordering logic (see above) is actually needed
  on this kernel version, or whether it silently clamps instead of
  erroring — verify empirically rather than assuming the general Linux
  behavior applies unchanged.

Resolved during design: SPI device discovery (glob, not hardcoded bus
number), PWM channel scope (0/1 only), GPIO blocklist extension (6-10,
18-19) — see sections above.
