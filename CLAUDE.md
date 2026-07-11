<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **Status: active.** The MT7688-side MCP server (`mt7688-mcp-server/`, 13 tools: GPIO/I2C/SPI/PWM/UART/I2S/ATmega bridge) and the ATmega32U4-side Arduino firmware (`atmega-firmware/`) are both implemented, deployed, and live-verified against real hardware — see [`README.md`](README.md) for the current status summary. The sections below document the *board architecture* (durable, load-bearing context) and the *standard toolchains*. A few sections are still marked **TODO** where genuinely nothing in this repo has needed that capability yet (e.g. raw MIPS C cross-compilation) — don't invent commands for those; confirm against actual files first if/when that work starts.

## What this board is

The LinkIt Smart 7688 **Duo** carries **two independent processors** that talk to each other over an internal serial link. Understanding the split is the whole architecture — most work targets one side, the "bridge" work targets both.

| | MT7688 side | ATmega32U4 side |
|---|---|---|
| Chip | MediaTek MT7688AN (MIPS24KEc @ 580 MHz) | Microchip ATmega32U4 (AVR @ **8 MHz**, 3.3V — not the common 16MHz/5V Leonardo default; confirmed via this board's `boards.txt` `build.f_cpu=8000000L`, verified 2026-07-11) |
| Runs | OpenWrt-based Linux ("LinkIt Smart 7688" firmware) | Bare-metal / Arduino firmware (Leonardo/Yún-class) |
| Role | Networking, WiFi, USB host, SD, compute, services | Real-time GPIO, ADC, sensors, PWM, timing |
| Code lives in | `mt7688-mcp-server/` (MCP server) | `atmega-firmware/` |
| Language | Python / Node.js / shell / C | Arduino C++ (`.ino` / C++) |
| Power | Shares the board's main power input | **Separate USB port** silkscreened "USB Power & MCU USB Port" (Micro-B, 5V IN) — the ATmega does not draw power from the MT7688 side or from the "USB Host" port. If ISP programming reports "AVR device not responding", check this port is connected before anything else. |

**The bridge is the point of the "Duo".** The two chips are wired together by a UART. Anything the Arduino side senses or actuates reaches the network/services only by passing bytes across this link, and vice versa. When designing or debugging, always ask *which side owns this* and *what crosses the bridge*.

- On the **ATmega32U4**, the hardware UART to the MT7688 is exposed as `Serial1` (the USB CDC port is `Serial`, used for the host/debug console).
- On the **MT7688 Linux** side, the same link appears as `/dev/ttyS0` — confirmed live 2026-07-11 via `dmesg`'s pinmux log (`rt2880-pinmux pinctrl: found group selector 10 for uart0` / `request pin 12 (io12) for 10000c00.uartlite` / `request pin 13 (io13) for 10000c00.uartlite`, cross-referenced against the boot line `10000c00.uartlite: ttyS0 at MMIO 0x10000c00 ... is a 16550A`) — not by elimination alone. This is a **different** physical UART from the one `uart_transact` (the shipped MCP tool) uses — that tool is fixed to `/dev/ttyS1` (pins 45/46, `uart1` pinmux group); the ATmega bridge is pins 12/13 (`uart0` group, device `10000c00.uartlite`). `/dev/ttyS2` (pins 20/21, `uart2` group) is the board's serial console (`console=ttyS2,57600`) — not available for other use.
- Bridge protocol: **Firmata**, ADC-read-only subset for v1 (`REPORT_ANALOG`/`ANALOG_MESSAGE`, channels 0-11 = A0-A11, 57600 baud) — see `docs/superpowers/specs/2026-07-11-atmega-phase-b-wire-protocol.md` for the full design and `atmega-firmware/adc-bridge/adc-bridge.ino` for the ATmega-side implementation.
- **The bridge is fully wired up end-to-end** (Phases A-D all done, live-verified 2026-07-11): `mt7688-mcp-server` exposes `atmega_adc_read(channel, timeout_ms) -> {value}` and `atmega_bridge_raw(tx_data, timeout_ms, max_read_bytes) -> {rx_data}` (Go source: `firmata.go`, `atmega.go`, `atmega_linux.go`/`atmega_other.go`; design: `docs/superpowers/specs/2026-07-11-atmega-phase-d-mcp-tools.md`). Both tools confirmed live against real hardware: `atmega_adc_read` returned real (floating-pin, jittering) 10-bit values; `atmega_bridge_raw` round-tripped raw `REPORT_ANALOG`/`ANALOG_MESSAGE` bytes matching Phase C's manual verification exactly.

## Working on the MT7688 (Linux/OpenWrt) side

Access is over SSH; the board default is user `root` at `mylinkit.local` (or `192.168.100.1` when the board is its own AP), confirmed live host `192.168.50.216`. Full credential/SSH-key setup (which key, why `plink` doesn't work, the four legacy-algorithm overrides this old Dropbear needs) is documented in [`provisioning/README.md`](provisioning/README.md) — don't re-derive it, read that file.

- **On-device package management:** `opkg update` / `opkg install <pkg>`. Runtimes commonly available: Python, Node.js, plus busybox userland — though nothing currently shipped in this repo actually needs an on-device runtime (`mt7688-mcp-server` is a statically-linked Go binary with `CGO_ENABLED=0`, no interpreter dependency). If future work needs a specific on-device runtime version, pin it here when that happens.
- **Deploy to board:** see "Run / test / lint" below for the MCP server's real `.\deploy.ps1` command; other code under this side should follow the same `scp`/SSH pattern until/unless it needs something different.
- **Cross-compiling C for MIPS** (only if native/opkg is insufficient): use the LinkIt 7688 / OpenWrt SDK toolchain (`mipsel-openwrt-linux-*`). Not yet needed by anything in this repo — `mt7688-mcp-server` uses Go's own built-in cross-compiler (`GOOS=linux GOARCH=mipsle`), no separate OpenWrt SDK/buildroot has ever had to be set up. TODO: document the toolchain path and build invocation if this project ever ships raw C.
- **Run / test / lint (MCP server, `mt7688-mcp-server/`):**
  - Test: `go test ./...` (single test: `go test ./... -run TestName -v`)
  - Build for the board: `.\build.ps1` (cross-compiles `GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0`)
  - Deploy: `.\deploy.ps1` (ships to `/usr/local/bin/mcp-server` via `scp -O`)

## Working on the ATmega32U4 (Arduino) side

This side is a standard Arduino target. The board package is **"LinkIt Smart 7688 Duo"**, FQBN **`LinkIt:avr:smart7688`** (confirmed live 2026-07-11).

**The board manager URL documented by the vendor (Seeed's wiki:
`http://download.labs.mediatek.com/package_mtk_linkit_smart_7688_index.json`)
is dead** — `labs.mediatek.com` no longer resolves in DNS at all (MediaTek
Labs' developer portal has been shut down). `arduino-cli`'s normal
"add board index → core install" flow does not work for this board anymore.
**Manual install workaround** (verified working 2026-07-11):
1. Install `arduino:avr` normally (`arduino-cli core install arduino:avr`) — this board's `platform.txt` depends on it for the base AVR core.
2. Clone `github.com/MediaTek-Labs/linkit-smart-7688-duo-arduino` at tag `0.1.7` (the latest real git tag; `0.1.8` was only ever published to the now-dead server).
3. Copy `mtk/avr/*` into `<arduino-cli data dir>/packages/LinkIt/hardware/avr/0.1.7/` (find the data dir via `arduino-cli config dump`).
4. `arduino-cli board listall | grep -i 7688` should then show `LinkIt Smart 7688 Duo    LinkIt:avr:smart7688`.

**This board's `pins_arduino.h` does not define `LED_BUILTIN`.** It does define `TXLED0`/`TXLED1`/`RXLED0`/`RXLED1` (standard Leonardo-family USB-activity-LED macros) but this specific board does not appear to populate that LED (compiled and flashed successfully, produced no visible blink). The board **does** have a real, populated LED — silkscreened "LED (D13)" on the board, near the MCU reset button — wired to plain Arduino pin **13** (use `pinMode(13, ...)`/`digitalWrite(13, ...)`, not a `LED_BUILTIN` alias). Confirmed both from the vendor's own `MediaTek LinkIt™ Smart 7688 Developer's Guide.pdf` (Figure 7 board photo, Figure 10 pin-out diagram — both in this repo root) and live hardware testing.

**Network/OTA upload is NOT the Yún serial-bootloader style this section previously assumed.** The board ships a complete **ISP-over-GPIO** flashing mechanism instead — no bootloader, no serial protocol, doesn't touch the bridge UART (`Serial1`/pins 12-13) at all:
- `/usr/bin/run-avrdude <hexfile> [extra-avrdude-args]` is pre-installed on the board. It temporarily repurposes GPIO pins 14-17 (the `spi_s` pinmux group) to plain GPIO and bit-bangs the standard 6-pin AVR ISP protocol via `avrdude -c linuxgpio`.
- Exact pin mapping (from the board's live `/etc/avrdude.conf`, cross-confirmed by the Developer's Guide's pin-out diagram showing MT7688 GPIO14-17 wired directly to the ATmega's hardware SPI pins D14-D17/SCLK/MOSI/MISO/SS): `reset=GPIO3, sck=GPIO15, mosi=GPIO16, miso=GPIO17`.
- **GPIO pin 3 is the ISP reset line** — this is also the pin this project's GPIO blocklist previously assumed was pin 38 based on indirect inference; that assumption looks wrong (see `docs/superpowers/specs/2026-07-11-atmega-phase-a-toolchain.md`'s "Live findings" for the full story) but hasn't been fixed in `mt7688-mcp-server/blocklist.go` yet — treat pin 3 as sensitive to the ATmega's reset state even though it's not currently blocklisted.
- Workflow: `arduino-cli compile --fqbn LinkIt:avr:smart7688 --output-dir <dir> <sketch-dir>` locally, then `scp` the resulting `.hex` to the board and immediately run `run-avrdude` — **don't let time pass between the two**, `/tmp` on this board is `tmpfs` and something on this board clears it periodically, so a `.hex` copied and not used within a few minutes may already be gone (`run-avrdude` will just report "No such file or directory").
- Upload over USB is also possible (`arduino-cli upload -p <port> --fqbn <fqbn> <sketch-dir>`) but wasn't used or verified in this project — TODO if wired upload is ever needed.
- **The ATmega side has its own separate power/USB port** (see board architecture table above) — if `avrdude` reports "AVR device not responding, initialization failed", check that port is connected before debugging anything else; ISP communication is impossible if the target chip has no power.
- **GUI alternative:** Arduino IDE with the same board selected — not set up or used in this project so far, `arduino-cli` is what's actually verified working.
- **Serial debug:** the ATmega's USB `Serial` is the console; `Serial1` is reserved for the bridge — don't print debug output to `Serial1` or it corrupts bridge traffic.

## Conventions for future work here

- **Keep the two sides in clearly separate top-level folders** and never blur their toolchains — MIPS/Linux and AVR/Arduino share nothing but the bridge protocol.
- **When adding a build/test/deploy command, add it to this file** (replacing the matching TODO) with the *exact* invocation, so it doesn't have to be rediscovered.
- **Bridge protocol changes are cross-cutting:** update both the ATmega encoder/decoder and the MT7688 counterpart in the same change, and note the wire-format version.
