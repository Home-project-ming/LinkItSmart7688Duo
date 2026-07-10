# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **Status: scaffold.** This directory is a fresh project for the Seeed **LinkIt Smart 7688 Duo**. No source, build files, or CI exist yet. The sections below document the *board architecture* (durable, load-bearing context) and the *standard toolchains*. Sections marked **TODO** must be filled in — and any command verified — once real code lands. Do not invent project-specific commands; confirm them against actual files first.

## What this board is

The LinkIt Smart 7688 **Duo** carries **two independent processors** that talk to each other over an internal serial link. Understanding the split is the whole architecture — most work targets one side, the "bridge" work targets both.

| | MT7688 side | ATmega32U4 side |
|---|---|---|
| Chip | MediaTek MT7688AN (MIPS24KEc @ 580 MHz) | Microchip ATmega32U4 (AVR @ 16 MHz) |
| Runs | OpenWrt-based Linux ("LinkIt Smart 7688" firmware) | Bare-metal / Arduino firmware (Leonardo/Yún-class) |
| Role | Networking, WiFi, USB host, SD, compute, services | Real-time GPIO, ADC, sensors, PWM, timing |
| Code lives in | `mt7688-mcp-server/` (MCP server) | (TODO: e.g. `firmware/`, `sketch/`) |
| Language | Python / Node.js / shell / C | Arduino C++ (`.ino` / C++) |

**The bridge is the point of the "Duo".** The two chips are wired together by a UART. Anything the Arduino side senses or actuates reaches the network/services only by passing bytes across this link, and vice versa. When designing or debugging, always ask *which side owns this* and *what crosses the bridge*.

- On the **ATmega32U4**, the hardware UART to the MT7688 is exposed as `Serial1` (the USB CDC port is `Serial`, used for the host/debug console).
- On the **MT7688 Linux** side, the same link appears as a `/dev/ttyS*` device. **Verify the exact node on-device** (`dmesg`, `ls /dev/ttyS*`) — do not assume; it depends on firmware version and DIP-switch/`uart` config.
- Define **one framing/protocol** for bridge traffic (line-delimited JSON, Firmata, or a custom binary frame) and keep both sides' encoders/decoders in sync. Treat the wire format as an API contract: a change on one side is a breaking change on the other.

## Working on the MT7688 (Linux/OpenWrt) side

Access is over SSH; the board default is user `root` at `mylinkit.local` (or `192.168.100.1` when the board is its own AP). TODO: record this project's actual host/credentials/network setup.

- **On-device package management:** `opkg update` / `opkg install <pkg>`. Runtimes commonly available: Python, Node.js, plus busybox userland. TODO: pin which runtimes/versions this project targets.
- **Deploy to board:** typically `scp`/`rsync` over SSH, or an on-device git checkout. TODO: record the real deploy command once chosen.
- **Cross-compiling C for MIPS** (only if native/opkg is insufficient): use the LinkIt 7688 / OpenWrt SDK toolchain (`mipsel-openwrt-linux-*`). TODO: document the toolchain path and build invocation if this project ships compiled binaries.
- **Run / test / lint (MCP server, `mt7688-mcp-server/`):**
  - Test: `go test ./...` (single test: `go test ./... -run TestName -v`)
  - Build for the board: `.\build.ps1` (cross-compiles `GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0`)
  - Deploy: `.\deploy.ps1` (ships to `/usr/local/bin/mcp-server` via `scp -O`)

## Working on the ATmega32U4 (Arduino) side

This side is a standard Arduino target. The board package is **"LinkIt Smart 7688 Duo"**; install it via the Arduino Boards Manager (it pulls the MediaTek/Seeed board definition). Sketches can be uploaded **over the network through the MT7688 bridge** (Yún-style) or over USB.

- **Build/upload (recommended, scriptable):** `arduino-cli`:
  - Compile: `arduino-cli compile --fqbn <fqbn> <sketch-dir>` — TODO: record the exact FQBN for this board once confirmed via `arduino-cli board listall | grep -i 7688`.
  - Upload over USB: `arduino-cli upload -p <port> --fqbn <fqbn> <sketch-dir>`.
  - Network upload (via MT7688): TODO — document the OTA/bridge upload command and target host.
- **GUI alternative:** Arduino IDE with the same board selected. TODO: note if this project standardizes on IDE vs. CLI.
- **Serial debug:** the ATmega's USB `Serial` is the console; `Serial1` is reserved for the bridge — don't print debug output to `Serial1` or it corrupts bridge traffic.

## Conventions for future work here

- **Keep the two sides in clearly separate top-level folders** and never blur their toolchains — MIPS/Linux and AVR/Arduino share nothing but the bridge protocol.
- **When adding a build/test/deploy command, add it to this file** (replacing the matching TODO) with the *exact* invocation, so it doesn't have to be rediscovered.
- **Bridge protocol changes are cross-cutting:** update both the ATmega encoder/decoder and the MT7688 counterpart in the same change, and note the wire-format version.
