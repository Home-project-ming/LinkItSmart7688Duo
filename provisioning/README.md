<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# provisioning/

Every script actually run against the board, in the order it was run. Kept so the
work is reproducible on a fresh board rather than rediscovered.

Narrative write-up with all the pitfalls: [`../dev-notes.html`](../dev-notes.html)

## Board

| | |
|---|---|
| Host | `192.168.31.156` / `mylinkit.local` (DHCP-assigned, moves when the board changes network — verify with `ping -4`) |
| Arch | mipsel 32-bit, soft-float (no FPU), MIPS 24KEc |
| Firmware | OpenWrt 15.05.1 Chaos Calmer, kernel 3.18.23 |
| vermagic | `3.18.23-1-32322c69cacfebf6e22f4e61cea9bebc` |
| System libc | uClibc 0.9.33.2 |
| SD | 116 GiB SDXC, ext4, UUID `a8a9d5f7-91ee-492f-b6c6-b16422b65f7a` |

## SSH key

`lib/keys/board_rsa` (private, RSA 2048, PEM format) / `board_rsa.pub` — this is the one that
actually works. `board_ed25519`/`.pub` also exist and are installed in
`/etc/dropbear/authorized_keys`, but **this Dropbear rejects ed25519 client keys** (very old
build — its host keys are RSA/DSS only, no ed25519/ecdsa at all) with a bare
"Permission denied", no useful error. RSA is the one to use.

**`plink` cannot use either key directly**, regardless of format (`new` or `-m PEM` old
format) — it insists on a puttygen-converted `.ppk`, and `puttygen` could not be driven
headlessly in this environment (it appears to need a GUI even for nominally-scriptable
invocations; repeated attempts produced no output and no file). Use the **native `ssh`**
client (already present on this machine at `/usr/bin/ssh`) instead, for key-based auth:

```powershell
ssh -i .\provisioning\lib\keys\board_rsa -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 '<command>'
```

All four legacy-algorithm overrides are required, each for a different negotiation phase
this 2016-era Dropbear needs but modern OpenSSH no longer offers by default:

| Flag | Why |
|---|---|
| `KexAlgorithms=+diffie-hellman-group14-sha1` | Dropbear's key exchange offer is SHA-1 only |
| `HostKeyAlgorithms=+ssh-rsa` | Dropbear's host key type is legacy `ssh-rsa`, not `rsa-sha2-*` |
| `PubkeyAcceptedAlgorithms=+ssh-rsa` | Dropbear only understands the legacy `ssh-rsa` client-key signature scheme, not `rsa-sha2-*` — without this, auth silently fails with "unknown algo" in the board's own logs, not a client-side error |
| `MACs=+hmac-sha1` | Dropbear only offers `hmac-sha1`/`hmac-md5` for the MAC, and neither is in modern OpenSSH's default-enabled MAC list — fails with `Unable to negotiate ... no matching MAC found`. **Client-dependent**: Windows' bundled `C:\Windows\System32\OpenSSH\ssh.exe` needs this explicitly; Git-Bash/MSYS's OpenSSH build enables `hmac-sha1` by default and doesn't strictly need it. Harmless to include regardless of which `ssh` resolves first, so always pass it. |

To (re)install the RSA public key on a fresh board (one-time, uses the still-working
password):

```powershell
$env:BOARD_PASSWORD = '<password>'
pscp -scp -batch -hostkey SHA256:pmSxVegIX1+q4Mkux0AB1hPZYylPT12RuMC0hAWLNa4 -pw $env:BOARD_PASSWORD .\lib\keys\board_rsa.pub root@192.168.31.156:/tmp/mcp_key_rsa.pub
.\lib\run-on-board.ps1 -Command 'mkdir -p /etc/dropbear && cat /tmp/mcp_key_rsa.pub >> /etc/dropbear/authorized_keys && chmod 600 /etc/dropbear/authorized_keys && rm /tmp/mcp_key_rsa.pub'
```

`run-on-board.ps1` itself still uses `plink`/`pscp` with the password — it hasn't been
switched to the key/native-ssh path. That's fine for scripted uploads (`-Script` mode), but
means the MCP server's own SSH invocation (its `command` in the client config) should use the
native `ssh` form above, not go through this script.

## Running a script

```powershell
$env:BOARD_PASSWORD = '<password>'
.\lib\run-on-board.ps1 -Script .\01-sd-extroot\preflight.sh
```

`run-on-board.ps1` exists because three things bite you otherwise: PowerShell
strips double quotes from native-exe arguments (so remote one-liners get mangled),
Dropbear has no `sftp-server` (so `pscp` needs `-scp`), and `plink` hangs on an
uncached host key (so the fingerprint gets pinned). It also converts CRLF to LF,
which `ash` requires.

## 01-sd-extroot/

Moves the root overlay from the 9.8 MB internal flash onto the SD card.

Run in this order:

| Script | What it does |
|---|---|
| `diag_sd.sh` | Why the SD won't mount. Reads the ext superblock by hand — the board has no `blkid`/`fdisk`/`od`. |
| `install_crc32c.sh` | Installs `kmod-crypto-crc32c` and mounts. **This is the fix**: the card has `metadata_csum`, and without crc32c the `mount` syscall hangs instead of erroring. |
| `check_extroot.sh` | Confirms `block-mount`'s dependencies are present. |
| `setup_extroot.sh` | Copies `/overlay` to the SD, writes `/etc/config/fstab`. |
| `sync_overlay.sh` | Re-copies `/overlay` **after** `fstab` exists. Needed because `setup_extroot.sh` writes `fstab` after the copy — fix the ordering if you rerun from scratch. |
| `check_bootmods.sh`, `check_preinit.sh` | Prove `ext4`/`crc32c` load in preinit, before `mount_root` needs them. |
| `preflight.sh` | Seven checks. Everything must pass before rebooting. |
| `verify_extroot.sh` | Run after reboot. |
| `check_mountd.sh`, `disable_mountd.sh`, `check_mountd_proc.sh` | `mountd` re-mounts the SD a second time and shares the whole rootfs over samba. Disable it. |
| `check_internal_fstab.sh` | Mounts the internal jffs2 read-only to show the `fstab` that preinit *actually* reads. |

`ipk/` holds the seven sideloaded packages. Sideloading is necessary because
`opkg update`'s signature check against the archive server fails, and the board's
busybox `wget` has no SSL.

### Two things that will bite later

**`fstab` exists twice.** extroot is decided in preinit, before the SD is mounted,
so `/sbin/block` reads `upper/etc/config/fstab` from the *internal* jffs2
(`/dev/mtdblock6`). The `/etc/config/fstab` you see once booted lives on the SD and
**does not affect boot**. To change boot behaviour, edit the internal copy.

**To undo extroot, pull the SD card.** `mount_root` falls back to the internal
overlay. The internal flash was never modified, only copied from.

## 02-python-entware/

`uv` has no MIPS build and needs Python ≥ 3.8; the OpenWrt feed tops out at 3.4.3.
Entware's `mipselsf-k3.4` target ships Python 3.13 against its own glibc 2.27 in
`/opt`, isolated from the system uClibc.

| Script | What it does |
|---|---|
| `check_python.sh` | Establishes arch, libc, FPU, existing Python. |
| `ent_pre.sh` | Prerequisites: `/opt` free, plain-HTTP reachable, RAM. |
| `generic.sh` | Entware's official installer, downloaded and reviewed before running. |
| `ent_python.sh` | `opkg install python3 python3-pip`. |
| `ent_verify.sh` | Read-only. Proves pip can reach PyPI over TLS. |
| `ent_path_venv.sh` | Appends `/opt/bin` to `PATH` in `/etc/profile`; installs `virtualenv` (Entware has no `python3-venv`). |
| `ent_finish.sh` | **Not run.** Also injected `rc.unslung` into `/etc/rc.local`; only needed if you install Entware *daemons*. |

## 03-diagnostics/

⚠️ **Entware binaries on the SD card are not reliable on this board.**

Any `/opt` binary crashes with SIGILL / SIGSEGV / SIGBUS once its pages have been
evicted from the page cache and must be faulted back in from the SD. With 126 MB
of RAM and no swap, that happens constantly.

Evidence (`test_triangulate.sh`, 10 cold-cache runs each):

```
a) loader + libs + exe on SD      ok=0/10
b) identical files in tmpfs       ok=10/10     <- binaries and glibc are fine
c) loader in RAM, libs+exe on SD  ok=0/10
```

`read()` of the same bytes is bit-perfect — `test_sd_reads.sh` md5s a 6.4 MB
library eight times with the cache dropped between each, and 48 MiB of the raw
partition four times. Every checksum matches. So the data reaching memory is
correct but the instructions executed are not, which points at I-cache/DMA
incoherency on the mmap page-fault path. **This was not proven down to the
driver**, and one result resists the theory: a 420 KB busybox executed from the
same SD passes 10/10.

Ruled out along the way: readahead (`test_readahead.sh`, `read_ahead_kb=0` changes
nothing), overlayfs (`test_overlayfs.sh`, the direct ext4 path fails too), a bad
card, and `virtualenv` itself (`diag_venv.sh`, `repro_sigill.sh`).

`test_exec_location.sh` is kept **as a broken test**: it copies busybox under a new
filename, and busybox dispatches on `argv[0]`, so every run returned 127 rather
than a signal. `test_exec_location2.sh` is the corrected version. Three separate
false positives showed up during this investigation — that one, `pgrep -f` matching
its own command line, and `$?` after a pipeline capturing `tail`'s exit code.

## 04-network-loop/

`eth0` (wired, to the home router) and `apcli0` (wireless client, same router)
ended up bridged into `br-lan` at the same time with no loop prevention —
plugging eth0 in took the whole LAN down, not just the board, within seconds.

| Script | What it does |
|---|---|
| `fix_bridge_stp.sh` | Enables STP on the `network.lan` bridge. **Didn't work** — `logread` kept showing `br-lan: received packet on eth0 with own address as source address` at 400-1000+/5s even 36s after link-up, no port ever reached `blocking`. Left enabled anyway; harmless. |
| `watch_stp.sh` | Polls `/sys/class/net/br-lan/brif/*/state` every 2s. This is what caught STP not converging, and later confirmed the actual bridge membership was `eth0` + `apcli0` (not `eth0` + `ra0`, which is what `network.lan.ifname` claims). |
| `move_apcli0_to_wan.sh` | **The actual fix.** `apcli0`'s bridge membership comes from `option network 'lan'` on its `wifi-iface` in `/etc/config/wireless`, not from `/etc/config/network` at all. Repoints it at the already-NAT'd `wan` network. Verified stable (no recurrence) after 9 minutes with eth0 connected.

Root cause note: STP can't fix this class of loop. It only prevents loops
between bridges that both speak STP and exchange BPDUs. Here the loop closes
through an ordinary home router that doesn't understand STP at all, so the
board's own bridge has no way to learn that `eth0` and `apcli0` are secretly
the same segment. The only real fix is topological — stop bridging one of the
two paths, route it instead.

## 05-squashfs-opt/

The fix experiment for 03's finding: pack `/opt` into a squashfs image on
the **same SD card** and loop-mount it. squashfs pages are decompressed by
the CPU into the page cache instead of DMA-filled on the mmap fault path —
the same reason the stock rootfs (`/rom`, squashfs on flash) never
misbehaved.

Result (2026-07-12): **ext4 control 0/20, squashfs 70/70**, all cold-cache.

| Script | What it does |
|---|---|
| `01_loop_support.sh` | Sideloads `kmod-loop` + `losetup` (+ `libsmartcols`) from `ipk/` — the stock kernel has no loop-device support at all. |
| `02_install_squashfs_tools.sh` | Installs Entware's `squashfs-tools-mksquashfs` (note the split package name — there is no plain `squashfs-tools`), with retries because Entware's own `opkg` is subject to the very bug being fixed. |
| `03_build_image.sh` | Builds `/overlay/opt.sqfs` (27.4 MB from 67.5 MB). Stages mksquashfs + the Entware loader + libs into tmpfs and runs it **from RAM** (the investigation's proven case-b workaround). **Must be `-comp xz`** — this kernel's squashfs has only the xz decompressor; a gzip image is rejected with `Filesystem uses "zlib" compression`. |
| `04_mount_and_probe.sh` | losetup + mount, then the exact `test_triangulate.sh` probe against identical binaries in both locations: ext4 grep 0/10 / python3 0/10 (history reproduced), squashfs grep 10/10 / python3 10/10, busybox-from-flash sanity 10/10. |
| `05_soak.sh` | 20 more cold rounds each for grep/python3 plus 10 rounds of a real Python computation with `PYTHONHOME` inside the image: 50/50. |
| `06_install_boot_hook.sh` | Makes the mount survive a reboot: appends a `losetup` + `mount -t squashfs` block to `/etc/rc.local` ahead of its `exit 0`, keyed on a marker comment so re-running is a no-op (`ALREADY_INSTALLED`). Backs up `rc.local` first, and every failure path only `logger`s and leaves `/opt` writable — a bad image can't cost you the boot or SSH. |
| `07_opt_maintenance.sh` | The write workflow for a read-only `/opt`. `unlock` unmounts and detaches the loop device to expose the real writable directory for `opkg`; `rebuild` regenerates `/overlay/opt.sqfs` from the current `/opt` and remounts it. `rebuild` refuses to run while `/opt` is still mounted, and stages mksquashfs into tmpfs the same way `03_build_image.sh` does. |

`ipk/` holds the three sideloaded packages (same archive.openwrt.org feed
and vermagic as `01-sd-extroot/ipk/`).

Made permanent 2026-07-14 (`06_`/`07_` above): verified by an actual
`reboot` — `/opt` came back as `squashfs (ro)` unattended, writes were
refused, and cold-cache `grep`/`python3` ran clean. Design:
`../docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md`.

## 06-xcs20/

Bring-up for driving the XCS20 FPGA on the attached 2000-era target board
over slave serial, through a 74AHCT125 level-shifting adapter into the
board's DB25. Design (wiring, protocol, risk register, stage-by-stage
procedure): `../docs/superpowers/specs/2026-07-30-xcs20-slave-serial-download-design.md`.

**Nothing here has been run against real hardware yet** — the adapter is
not built.

| Script | What it does |
|---|---|
| `01_manual_probe.sh` | Single-steps the slave-serial signals so a human with a DMM can walk stages 3-4 of the design doc before any Go code runs. busybox-only. Subcommands: `init` / `status` / `gate on\|off` / `set din\|cclk\|prog 0\|1` / `clock N [SEC]` / `pulse-program` / `release`. Refuses blocklisted or duplicated pins, parks the buffer gate high (Hi-Z) on `init` and `release`, and exits non-zero if `pulse-program` fails to clear DONE. |

The actual download is the `fpga_configure` MCP tool
(`../mt7688-mcp-server/fpga.go`), driven from the host by the sister
repo's `build.ps1 -Download`. Order on the bench: `01_manual_probe.sh`
stages first, then `-Download -DryRun`, then a real download.

Pin numbers are arguments everywhere, never constants — which 7688 GPIOs
reach the physical header still has to be confirmed with a meter.
