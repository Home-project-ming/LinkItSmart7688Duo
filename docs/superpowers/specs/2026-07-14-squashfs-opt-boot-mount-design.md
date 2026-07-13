<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# squashfs `/opt` boot-mount + rebuild workflow — design

Status: approved, not yet implemented
Date: 2026-07-14

## Context

`provisioning/05-squashfs-opt/` proved (2026-07-12, 70/70 cold-cache runs)
that packing `/opt` (Entware, SD ext4-backed) into a squashfs image and
loop-mounting it avoids the SIGILL/SIGSEGV crashes documented in
`provisioning/README.md`'s 03-diagnostics section — squashfs pages are
CPU-decompressed into the page cache instead of DMA-filled on the mmap
page-fault path, the same reason the stock rootfs (squashfs-on-flash)
never misbehaved.

That mount is currently manual (`04_mount_and_probe.sh`) and does not
survive a reboot — confirmed live 2026-07-14: after the 07-12 test
session's reboot, `mount | grep opt` and `lsmod | grep loop` needed
re-verification and nothing was mounted. `dev-notes.html` has stood since
then with an explicit ban — "不要把 /opt 裡的東西放進開機自動啟動的服務" —
with a note that the ban has an exit once boot automation lands. This spec
is that exit: TODO.md's top-priority item.

**Live state confirmed before writing this spec** (2026-07-14, via SSH):
- `kmod-loop` and `libsmartcols` are still opkg-installed (persist across
  reboots — they live in the SD-backed overlay, not tmpfs).
- `/etc/modules.d/30-loop` already exists, meaning OpenWrt's `kmodloader`
  auto-loads the `loop` module at boot — confirmed via `lsmod`. **No
  manual `insmod` is needed in the boot hook.**
- `/dev/loop0`-`loop7` device nodes already exist (devtmpfs).
- `/overlay/opt.sqfs` (28.7MB, built 2026-07-12) is still present but
  stale relative to whatever `/opt` has changed since.
- Nothing is currently mounted at `/opt` or via any loop device — the
  07-12 mount did not survive that session's reboot, as expected.

## Goals

1. `/opt` is automatically loop-mounted read-only from `/overlay/opt.sqfs`
   on every boot, with no manual step.
2. A safe, repeatable workflow exists for updating `/opt` (opkg
   install/upgrade) and folding those changes back into the image.
3. Once (1) is verified live across a real reboot, lift the dev-notes.html
   ban on auto-starting `/opt`-based services.

## Non-goals

- Actually wiring up an Entware daemon's autostart (e.g. `rc.unslung`).
  TODO.md only asks to lift the *ban*, not to add a specific service —
  that's separate future work once someone actually needs a daemon
  running from `/opt` at boot.
- Preinit-level integration. Rejected in favor of `/etc/rc.local` — see
  "Where the boot hook lives" below.
- Automating the `opkg install`/`upgrade` step itself. Which packages to
  install is a human decision made per-occasion; the tooling here only
  automates the mechanical parts around it (exposing `/opt` as writable,
  rebuilding the image, remounting).
- Supporting services that need a writable subdirectory somewhere under
  `/opt` at runtime (state, sockets, logs, etc.) — a plain read-only mount
  can't do that. Nothing today needs this (nothing auto-starts from
  `/opt` yet, per the ban this spec lifts). If a future daemon needs
  runtime write access under `/opt`, that's a follow-on design (e.g. an
  overlayfs tmpfs upper layer), not something this spec's plain `-o ro`
  mount provides.

## Where the boot hook lives: `/etc/rc.local`, not preinit

`/etc/rc.local` runs after the entire OpenWrt boot sequence completes
(all of `/etc/init.d/*`, including `block-mount`/extroot), and **its exit
status is never checked** — the file's own default content ends in a bare
`exit 0`. This gives a specific, load-bearing safety property: if the
`losetup`/`mount` commands in the hook fail for any reason (corrupt image,
SD not ready, loop device busy), boot still completes normally and SSH
access is never at risk. The worst case is `/opt` staying exactly as it
behaves today — a real directory on the SD ext4 overlay. The hook only
*adds* a mount on top of that; it never removes, moves, or reformats
anything, so there is no failure mode that loses data.

This is a deliberately different risk profile from `01-sd-extroot/`'s
preinit work, where getting the fstab/preinit ordering wrong could affect
whether the board mounts its root filesystem at all. Nothing here touches
preinit or `mount_root`.

## Boot hook implementation

Appended to `/etc/rc.local`, before the existing `exit 0`:

```sh
# squashfs /opt boot mount — see docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md
if ! mount | grep -q ' /opt '; then
    if [ -f /overlay/opt.sqfs ]; then
        losetup /dev/loop0 /overlay/opt.sqfs 2>/tmp/opt-sqfs-boot.log \
            && mount -t squashfs -o ro /dev/loop0 /opt 2>>/tmp/opt-sqfs-boot.log \
            || logger -t opt-sqfs "boot mount failed, /opt stays writable - see /tmp/opt-sqfs-boot.log"
    else
        logger -t opt-sqfs "/overlay/opt.sqfs missing, /opt stays writable"
    fi
fi
```

- Idempotency guard (`mount | grep`) matches the existing style in
  `04_mount_and_probe.sh` rather than introducing `mountpoint` (not
  confirmed present in this BusyBox build).
- Hardcodes `/dev/loop0`, matching every prior script in this directory —
  nothing else on this board uses loop devices, so no collision risk.
- Logs to both a `/tmp` file (survives until next reboot, `/tmp` is
  tmpfs) and `logger` (reaches `logread`) on failure, so a failed mount
  is discoverable without having to already know to look for it.

Installed via a new idempotent installer script (checks for the marker
comment before appending, so re-running `deploy` doesn't duplicate the
block) rather than hand-editing `/etc/rc.local` over SSH each time,
matching this project's existing "script everything actually run against
the board" convention (`provisioning/README.md`'s stated purpose).

## Rebuild-when-/opt-changes workflow

`/opt` being read-only (mounted from squashfs) is incompatible with
`opkg install`/`upgrade` targeting `/opt` directly — the mount has to come
off first, exposing the real writable ext4 directory underneath (mounting
shadows a directory, it doesn't delete it — the real files are untouched
and reappear the moment the mount is removed).

One script, two subcommands — `provisioning/05-squashfs-opt/06_opt_maintenance.sh`:

- **`unlock`**: `umount /opt` (if mounted) + `losetup -d /dev/loop0` (if
  attached). After this, `/opt` is the plain writable ext4 directory again
  and normal `opkg install`/`upgrade` works.
- **`rebuild`**: rebuilds `/overlay/opt.sqfs` from the current `/opt`
  (identical mksquashfs-from-tmpfs approach as `03_build_image.sh` — copy
  the Entware loader/libs/mksquashfs binary into `/tmp` first, since
  `mksquashfs` itself is an Entware glibc binary subject to the same
  cold-cache crash the whole image exists to avoid), then
  `losetup`+`mount`s it back onto `/opt`.

Usage: `unlock` → do whatever `opkg` operations by hand → `rebuild`. The
two subcommands are deliberately separate (not one atomic "update"
command) because the `opkg` step in between is a human decision with
variable duration and content — scripting it away would mean guessing
what the caller wants to install.

## Verification plan

1. Deploy the `rc.local` installer + `06_opt_maintenance.sh` via the
   existing `run-on-board.ps1 -Script` pattern.
2. Confirm `rc.local` now contains the marker block (no reboot yet).
3. **Actual `reboot`** of the board (confirmed acceptable given the
   non-blocking failure mode established above).
4. After the board comes back and SSH is reachable again:
   - `mount | grep /opt` shows the squashfs loop mount.
   - A write attempt against `/opt` fails (read-only filesystem) —
     confirms it's really the squashfs mount, not a stale directory.
   - A couple of the same cold-cache-style probes from
     `04_mount_and_probe.sh` (`grep --version`, `python3 --version` via
     the squashfs path) still succeed, confirming the mount isn't just
     present but actually usable.
5. Exercise `06_opt_maintenance.sh unlock` then `rebuild` once, end to
   end, with no actual package changes (rebuilding from an unmodified
   `/opt` is a valid, cheap way to prove the mechanism works without
   needing a real opkg operation as part of this verification pass).
6. Only after steps 3-5 all pass: update `dev-notes.html` to remove/amend
   the autostart ban, and check off all three TODO.md items under a new
   ✅ block (matching the existing completed-item style), noting the
   result the way the 07-12 squashfs experiment entry does.

## Rollback

Nothing here is destructive. If step 4 or 5 fails, the fix is either
`umount /opt` by hand (falls back to today's behavior) or removing the
appended `rc.local` block and rebooting again — no data loss in any
failure branch, since the underlying writable `/opt` is never modified or
deleted by any part of this design.
