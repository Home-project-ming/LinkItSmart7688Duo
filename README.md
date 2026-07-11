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
coding agent (Claude Code) drive the board's GPIO and I2C hardware directly
through MCP tools instead of hand-typed SSH sessions.

## What's here

| Path | What it is |
|---|---|
| [`dev-notes.html`](dev-notes.html) | Narrative write-up: moving the root filesystem from 9.8 MB of internal flash onto a 116 GiB SD card, getting a modern Python running on 2016 firmware, and an unresolved SIGILL/SIGSEGV bug in SD-resident dynamically-linked binaries |
| [`provisioning/`](provisioning/) | Every shell script actually run against the board, kept in the order it was run so the work is reproducible on a fresh board rather than rediscovered. See [`provisioning/README.md`](provisioning/README.md) |
| [`mt7688-mcp-server/`](mt7688-mcp-server/) | The MCP server itself — Go, cross-compiled for the board, exposing `gpio_read`/`gpio_write`/`gpio_set_direction`/`i2c_read`/`i2c_write` as tools an MCP client can call |
| [`docs/superpowers/specs/`](docs/superpowers/specs/) | Design docs — the reasoning behind architectural decisions, including ones that were revised mid-implementation |
| [`docs/superpowers/plans/`](docs/superpowers/plans/) | Task-by-task implementation plans |
| [`CLAUDE.md`](CLAUDE.md) | Instructions for AI coding agents working in this repo — board architecture, toolchains, real build/test/deploy commands |

## The board

| | MT7688 side | ATmega32U4 side |
|---|---|---|
| Chip | MediaTek MT7688AN, MIPS24KEc @ 580 MHz | Microchip ATmega32U4, AVR @ 16 MHz |
| Runs | OpenWrt 15.05.1 (Chaos Calmer), kernel 3.18.23 | Bare-metal / Arduino firmware |
| Role | Networking, WiFi, USB host, SD, services | Real-time GPIO, ADC, PWM |

The two chips are wired together over UART — the "Duo" in the name is the
whole point. Anything the AVR side senses or actuates reaches the network
only by crossing that bridge, and vice versa.

## Highlights

- **Extroot done**: root overlay moved from 9.8 MB internal flash to a 116 GiB SD card (~11,000x more space), survives reboots.
- **Modern Python on 2016 firmware**: Entware's `mipselsf-k3.4` target runs Python 3.13 against its own glibc, isolated from the system's uClibc — with a documented, unresolved reliability caveat (see `dev-notes.html`).
- **A real network bridge loop, found and fixed**: the wireless uplink was accidentally bridged onto the same L2 segment as the wired port with STP disabled — diagnosed live on the board, not guessed at.
- **A working MCP server**, deployed to and verified against the real board: GPIO and I2C tools, a hardcoded safety blocklist (console UART, SD/extroot control lines, the ATmega bridge UART, the ATmega's ISP reset line) checked before any hardware access, and two real bugs (a concurrent-export race, a Go `[]byte`-vs-JSON-array serialization mismatch) that only surfaced under live testing and are now fixed with regression coverage.

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

Actively evolving. The MCP server exposes 10 tools: GPIO (read/write/
direction), I2C (read/write), SPI (`spi_transfer`), PWM (`pwm_configure`/
`pwm_set_enabled`/`pwm_read`), and UART (`uart_transact`). Structurally
verified live against real hardware except `i2c_write` (code review + unit
test only, no I2C device safely available to test a write against) and
`uart_transact`'s byte-level round-trip correctness (no real serial device
or loopback available — the call succeeding and reading real bytes from an
unconnected line was confirmed, full correctness against a peer device
wasn't). I2S, the ATmega UART bridge, and ADC are not yet exposed as tools
— ADC specifically has no MT7688-side hardware path at all (confirmed live:
no IIO/ADC subsystem exists on this chip) and depends on the ATmega bridge
work landing first. See `docs/superpowers/specs/2026-07-11-uart-design.md`'s
roadmap table for the full picture and why each is scoped separately.

## Copyright

Copyright (c) 2026 Ming-Chih Lung (Taiwan, Tainan). See individual files
for scope — third-party content (the MediaTek developer's guide PDF,
sideloaded OpenWrt packages) is excluded.
