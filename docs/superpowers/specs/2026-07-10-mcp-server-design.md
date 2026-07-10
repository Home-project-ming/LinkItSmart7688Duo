<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# MCP server for board hardware control — design

Status: implemented, verified live on real hardware 2026-07-10
Date: 2026-07-10

## Live verification (2026-07-10)

All five tools deployed to the board and exercised through a real MCP client
session (not just `go test`):

- All five tools (`gpio_read`, `gpio_write`, `gpio_set_direction`,
  `i2c_read`, `i2c_write`) visible and callable.
- GPIO blocklist confirmed live: `gpio_read` on pin 21 (console) and pin 12
  (ATmega bridge UART) both correctly rejected with clear errors.
- GPIO read/write round-trip confirmed on pin 3 (silkscreened `D13LED` on
  the MT7688 side, distinct from the ATmega's own onboard LED on its own
  D13 — a real naming collision between the two chips worth remembering).
  Visually confirmed by the user: the LED is active-low (`value: false`
  drives it on, `value: true` off) — write→read round-tripped consistently
  with what was observed.
- `i2c_read` confirmed live against a real address on `/dev/i2c-0`. This
  caught a real bug (below) on first live use.
- `i2c_write` is verified by code review and unit test only, not live —
  no I2C device is attached to test against safely, and guessing at an
  address to write real data to (e.g. `0x50`, a common EEPROM address) was
  correctly refused rather than risking overwriting unknown device state.
  Uses the identical fix pattern as the now-live-verified read path. Treat
  as a known gap, not a blocker — first real `i2c_write` usage against an
  actual attached device is the remaining verification step.

**Bug found and fixed during live verification:** `I2CReadOutput.Data` and
`I2CWriteInput.Data` were typed `[]byte`. Go's `encoding/json` always
marshals/unmarshals `[]byte` as a base64 string, never a JSON array — a
hardcoded special case the schema generator doesn't account for, so the
auto-generated schema promised `"type": "array"` while the actual encoder
produced a string. This didn't surface in `go test` (which never exercises
real MCP JSON serialization) or in earlier validation-only live calls
(which return before reaching a real data payload) — only appeared once
`i2c_read` returned actual non-empty data from real hardware. Fixed by
retyping both fields to `[]int` and converting to/from `[]byte` at the
`main.go` boundary (`i2c.go` itself is untyped and untouched), with an
explicit 0-255 range check on the write path to avoid reintroducing the
same silent-truncation failure mode the `addr`/`reg` fix (Task 5) already
closed once this session.

## Context

The board (MT7688 side, OpenWrt) has had its GPIO/I2C/SPI/PWM/UART/I2S surface
mapped out through manual SSH sessions (see `dev-notes.html` and
`provisioning/`). The goal now is to let an MCP client (Claude Code, running
on the Windows PC that has been driving this whole investigation) call tools
that operate this hardware directly, instead of hand-running shell scripts
over SSH each time.

An earlier plan assumed the server would need to be a resident daemon,
because the only viable modern Python (Entware, glibc, installed on the SD
card via extroot) crashes with SIGILL/SIGSEGV/SIGBUS on ~0/10 cold-cache
runs — anything paged in from the SD after eviction comes back corrupted,
suspected I-cache/DMA incoherency on the mmap page-fault path, not proven to
driver level. A tmpfs staging workaround was validated (10/10) but never
implemented, and residency was meant to dodge repeated cold starts.

That constraint doesn't apply to this design: see "Language and placement"
below.

## Goals (v1)

- Expose GPIO read/write/direction and I2C read/write as MCP tools.
- Safe by construction: known-dangerous GPIO pins are refused, not just
  documented.
- No new attack surface: reuse the SSH key auth already set up for this
  project rather than inventing a network auth scheme.

## Non-goals (v1)

- SPI, PWM, the ATmega UART bridge, I2S. Deferred until the GPIO/I2C
  primitives and the overall shape have been exercised for real.
- Device-specific abstractions (e.g. a named sensor driver). v1 exposes raw
  bus primitives only.
- A persistent network listener. See "Transport" below.

## Language and placement

Go, cross-compiled `GOOS=linux GOARCH=mipsle GOMIPS=softfloat`, statically
linked (default for CGO_ENABLED=0), deployed to a normal path on the
**SD-backed overlay** (e.g. `/usr/local/bin/`) via plain `pscp` — the same
mechanism every other file in this project already gets deployed with.

Rationale: the SIGILL/SIGSEGV crash pattern documented in `dev-notes.html`
was demonstrated specifically for Entware's dynamically-linked glibc
binaries (separate loader + .so files, each independently mmap'd and
faulted in from SD). The one clean counter-example in that investigation —
busybox, statically linked, executed from the same SD — passed 10/10. A
static Go binary matches that working case. The deciding variable is
static-vs-dynamic linking, not storage medium.

*(Revision, made while writing the implementation plan: an earlier version
of this section also called for placing the binary on internal flash, on
top of static linking, for "extra margin." That was wrong on two counts.
First, the evidence doesn't support treating flash-vs-SD as the safety
factor — busybox's reliability tracks with squashfs's read-only,
CPU-decompressed page-in behavior, not "flash" as a storage medium in
general, and that reasoning doesn't transfer to writing to a jffs2 overlay
mount. Second, and more fundamently, it wouldn't have worked mechanically:
with extroot active, the internal jffs2 overlay's `upper/` is only what
boots from if the SD is absent — writing a file there while the SD is
present and mounted puts it somewhere the running system never reads.
Plain SD-backed deployment is both simpler and the only path actually
grounded in evidence.)*

This also removes the original motivation for a resident daemon — a cold
start of a static binary isn't expected to hit the crash pattern regardless
of which writable storage it's deployed to, so there's no need to dodge
repeated cold starts.

## Transport

SSH-spawned stdio, not a persistent network listener. The MCP client's
`command` is native `ssh` (not `plink` — see below) with the RSA key and
three legacy-algorithm overrides this board's old Dropbear requires:

```
ssh -i provisioning/lib/keys/board_rsa -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa root@192.168.50.216 /path/to/mcp-server
```

Each MCP session starts a fresh process on the board over that SSH
connection and it exits when the session ends.

Verified end-to-end 2026-07-10. Things worth recording so they aren't
rediscovered: this Dropbear rejects ed25519 client keys outright (use the
RSA key, not the ed25519 one also sitting in `provisioning/lib/keys/`), and
`plink` cannot load either key directly in any format — it wants a
puttygen-converted `.ppk`, and puttygen couldn't be driven headlessly here.
Native `ssh` works with the OpenSSH-format key as-is; use that, not `plink`,
for this specific connection.

A 4th override, `-o MACs=+hmac-sha1`, was found necessary during Task 6's
real hardware deploy — client-dependent: Windows' bundled OpenSSH
(`OpenSSH_for_Windows_9.5p2`, what a plain `ssh`/`scp` resolves to from
PowerShell) doesn't enable `hmac-sha1` by default and fails to negotiate
with this Dropbear without it; Git-Bash/MSYS OpenSSH does enable it by
default and works without the override. Since which `ssh` binary actually
runs depends on PATH resolution, include this 4th flag unconditionally —
it's harmless to pass even when not strictly needed. Full detail in
`provisioning/README.md`.

Rejected alternative: a resident daemon listening on a TCP port. Would need
its own auth layer (anyone reaching the port could operate hardware),
its own start-on-boot and crash-restart handling, and a port to manage.
None of that is needed once cold-start reliability isn't a concern — SSH's
existing key auth for this project covers the auth requirement for free.

## Tool surface (v1)

| Tool | Signature | Notes |
|---|---|---|
| `gpio_read` | `(pin int) -> value` | |
| `gpio_write` | `(pin int, value bool)` | |
| `gpio_set_direction` | `(pin int, direction "in"\|"out")` | |
| `i2c_read` | `(addr int, reg int, len int) -> bytes` | fixed to `/dev/i2c-0` |
| `i2c_write` | `(addr int, reg int, data []byte)` | fixed to `/dev/i2c-0` |

## Safety: GPIO blocklist

Every `gpio_*` tool checks the pin against a hardcoded blocklist before
touching hardware, and returns a clear error (not a silent no-op) if the
pin is blocked:

- `20`, `21` — console
- `22`–`29` — SDXC / extroot
- `38` — ATmega ISP reset (held by the core)

This list comes directly from the hardware survey already done on this
board; it is not being derived fresh here.

## Build & deploy

Cross-compile locally on the Windows PC (`go build` with the env vars
above). Deploy with native `scp -O` (legacy protocol, same reason
`pscp -scp` is used elsewhere — this Dropbear has no `sftp-server`) using
the RSA key rather than `pscp`+password: deployment happens repeatedly
during development, so it's worth it here specifically, unlike the
one-off `provisioning/` scripts. Destination is `/usr/local/bin/` on the
board — a normal SD-backed path, no special mount handling needed (see
revision note above).

Server source lives in a new top-level directory (fills in the `Code lives
in (TODO: ...)` line in `CLAUDE.md` for the MT7688 side), separate from
`provisioning/`, which stays scoped to one-off setup/diagnostic scripts.
Exact directory name and the build/deploy command get pinned down (and
`CLAUDE.md` updated) during implementation, per this repo's own convention
of not documenting a command until it's been run for real.

## Testing

- Pin blocklist logic and request validation: plain `go test`, runs on the
  Windows PC, no board needed.
- Actual GPIO/I2C reads and writes: can't be meaningfully tested off the
  hardware. Manual verification against the real board after deploy.

## Open questions for implementation

- Exact Go MCP SDK to depend on — pick during implementation, verify it
  still builds cleanly for `mipsle`/softfloat before committing to it.
- Exact internal-flash path to deploy the binary to, and whether it needs
  an `/etc/init.d` entry at all given it's spawned per-SSH-session rather
  than running as a service.

Resolved during design: SSH transport (native `ssh` + RSA key + three
legacy-algorithm overrides) verified working end-to-end — see "Transport"
above.
