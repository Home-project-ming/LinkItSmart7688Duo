<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# LinkIt Smart 7688 Duo

[繁體中文](README.zh-TW.md)

Hardware bring-up notes and an MCP (Model Context Protocol) server for the
[Seeed / MediaTek LinkIt Smart 7688 Duo](https://www.seeedstudio.com/LinkIt-Smart-7688-Duo.html) —
a dev board carrying two independent processors: a MediaTek **MT7688AN**
(MIPS24KEc, OpenWrt Linux) and an Atmel **ATmega32U4** (bare-metal AVR),
bridged over UART.

This repo is both a written record of getting real work done on a
resource-constrained, 2016-era board, and a working tool that lets an AI
coding agent (Claude Code) drive the board's hardware directly through MCP
tools instead of hand-typed SSH sessions — GPIO, I2C, SPI, PWM, UART, I2S
audio, and (via a second onboard processor) analog input.

## What's here

| Path | What it is |
|---|---|
| [`dev-notes.html`](dev-notes.html) | Narrative write-up of the whole bring-up: SD-card extroot, modern Python on 2016 firmware (with an unresolved SIGILL/SIGSEGV bug in SD-resident dynamically-linked binaries), a network bridge loop, and the traps hit while building the MCP server and waking the ATmega32U4 |
| [`applications.html`](applications.html) | An illustrated brainstorm (in Chinese): 50 real-life applications for this board + MCP server, organized by DIY skill level — from solder-your-own-circuits, through wire-up-store-bought-modules, to zero-wiring always-on network services — each idea feasibility-checked against the board's real constraints |
| [`provisioning/`](provisioning/) | Every shell script actually run against the board, kept in the order it was run so the work is reproducible on a fresh board rather than rediscovered. See [`provisioning/README.md`](provisioning/README.md) |
| [`mt7688-mcp-server/`](mt7688-mcp-server/) | The MCP server itself — Go, cross-compiled for the board, exposing 13 hardware-primitive tools (GPIO/I2C/SPI/PWM/UART/I2S/ATmega bridge) an MCP client can call |
| [`atmega-firmware/`](atmega-firmware/) | Arduino firmware for the board's second processor (ATmega32U4) — `adc-bridge/` speaks a Firmata subset over the inter-chip UART so the MT7688 side can read the ATmega's analog inputs |
| [`docs/superpowers/specs/`](docs/superpowers/specs/) | Design docs — the reasoning behind architectural decisions, including ones that were revised mid-implementation |
| [`docs/superpowers/plans/`](docs/superpowers/plans/) | Task-by-task implementation plans |
| [`CLAUDE.md`](CLAUDE.md) | Instructions for AI coding agents working in this repo — board architecture, toolchains, real build/test/deploy commands |
| [`.claude/skills/mt7688-hardware/`](.claude/skills/mt7688-hardware/) | An agent skill for using the deployed MCP tools effectively — when to reach for which tool, safety limits, and gotchas that aren't obvious from a tool's schema alone |

## The board

| | MT7688 side | ATmega32U4 side |
|---|---|---|
| Chip | MediaTek MT7688AN, MIPS24KEc @ 580 MHz | Microchip ATmega32U4, AVR @ 8 MHz, 3.3V |
| Runs | OpenWrt 15.05.1 (Chaos Calmer), kernel 3.18.23 | Bare-metal / Arduino firmware |
| Role | Networking, WiFi, USB host, SD, services | Real-time GPIO, ADC, PWM |

The two chips are wired together over UART — the "Duo" in the name is the
whole point. Anything the AVR side senses or actuates reaches the network
only by crossing that bridge, and vice versa. That bridge is now fully
wired up end to end: `atmega-firmware/adc-bridge/` runs a small Firmata
subset on the ATmega, and the MCP server's `atmega_adc_read`/
`atmega_bridge_raw` tools speak it from the Linux side — this is also
where analog input access lives, since the MT7688 itself has no ADC
hardware at all.

## Highlights

- **Extroot done**: root overlay moved from 9.8 MB internal flash to a 116 GiB SD card (~11,000x more space), survives reboots.
- **Modern Python on 2016 firmware**: Entware's `mipselsf-k3.4` target runs Python 3.13 against its own glibc, isolated from the system's uClibc — with a documented, unresolved reliability caveat (see `dev-notes.html`).
- **A real network bridge loop, found and fixed**: the wireless uplink was accidentally bridged onto the same L2 segment as the wired port with STP disabled — diagnosed live on the board, not guessed at.
- **A working MCP server**, deployed to and verified against the real board: 13 tools spanning GPIO/I2C/SPI/PWM/UART/I2S/ATmega bridge, a hardcoded safety blocklist (console UART, SD/extroot control lines, the ATmega bridge UART, ISP lines) checked before any hardware access, and several real bugs (a concurrent-export race, a Go `[]byte`-vs-JSON-array serialization mismatch, a silently-swallowed error that could leave the ATmega bridge stuck mid-protocol) that only surfaced under live testing or code review and are now fixed with regression coverage.
- **A second processor, bridged end to end**: the ATmega32U4 runs Arduino firmware speaking a minimal Firmata subset over its own dedicated UART (confirmed live as `/dev/ttyS0` on the Linux side, distinct from the general-purpose `uart_transact` UART), giving the MCP server real analog-input access that the MT7688 chip itself doesn't have.

## Using the MCP server

```powershell
cd mt7688-mcp-server
go test ./...      # unit tests, no hardware needed
.\build.ps1         # cross-compiles for the board (mipsel, soft-float, static)
.\deploy.ps1        # ships the binary to the board over SSH
```

An MCP client (e.g. Claude Code) connects by spawning the deployed binary
over SSH per session — see [`.mcp.json`](.mcp.json) for the exact command
and [`provisioning/README.md`](provisioning/README.md) for why this
particular board's SSH connection needs four legacy-algorithm overrides
that a normal modern OpenSSH client doesn't offer by default.

## Status

The originally planned hardware-coverage roadmap (GPIO/I2C → SPI/PWM →
UART → I2S → ATmega bridge → ADC) is complete. The MCP server exposes 13
tools: GPIO (read/write/direction), I2C (read/write), SPI
(`spi_transfer`), PWM (`pwm_configure`/`pwm_set_enabled`/`pwm_read`), UART
(`uart_transact`), I2S (`i2s_play`), and the ATmega32U4 bridge
(`atmega_adc_read`, `atmega_bridge_raw`). Structurally verified live
against real hardware except `i2c_write` (code review + unit test only,
no I2C device safely available to test a write against),
`uart_transact`'s byte-level round-trip correctness (no real serial
device or loopback available), and `i2s_play`'s audible output (no
speaker was connected during verification — the call completing without
error was confirmed, whether it actually produced clean audio wasn't).
I2S recording is confirmed **unavailable**, not just unimplemented: the
board's capture path has a reproducible driver-level bug that hangs the
audio device (`SIGKILL`-resistant, requires a reboot) — found during
design and re-confirmed when a buffer-size workaround was tried and also
failed — see `docs/superpowers/specs/2026-07-11-i2s-design.md`'s
"Investigation" section. The ATmega UART bridge is fully wired up
end-to-end across four phases (toolchain bring-up, Firmata wire-protocol
design, ATmega firmware, MT7688-side decoder + MCP tools — see
`docs/superpowers/specs/2026-07-11-atmega-phase-b-wire-protocol.md`,
`docs/superpowers/specs/2026-07-11-atmega-phase-d-mcp-tools.md`, and the
matching plan docs), live-verified against real hardware including a
raw-byte round-trip decode of the Firmata protocol. There is one still-open correctness question tracked in `CLAUDE.md`:
whether the GPIO blocklist's "ATmega ISP reset" entry is on the right pin
number (real evidence points to pin 3, the blocklist currently says 38) —
flagged, not yet fixed. This is worth fixing sooner rather than later now
that the ATmega bridge is live: an MCP caller toggling GPIO 3 thinking
it's "just an LED" would reset the ATmega mid-operation, since real
firmware (and real MCP tools depending on it) now exists for a stray
reset to actually disrupt.

## Copyright

Copyright (c) 2026 Ming-Chih Lung (Taiwan, Tainan). See individual files
for scope — third-party content (the MediaTek developer's guide PDF,
sideloaded OpenWrt packages) is excluded.
