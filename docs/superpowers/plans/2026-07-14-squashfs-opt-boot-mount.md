<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# squashfs `/opt` Boot-Mount + Rebuild Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the proven squashfs `/opt` loop-mount workaround (70/70 cold-cache runs, `provisioning/05-squashfs-opt/`) survive a reboot automatically, give it a safe update workflow, and lift the dev-notes.html ban on `/opt`-based autostart services — per `docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md`.

**Architecture:** This is infrastructure bring-up on real embedded hardware, not application code — there is no unit-test cycle here. The "test" at every step is a real command run against the live board over SSH, with an exact expected output. Two new shell scripts join the existing numbered scripts in `provisioning/05-squashfs-opt/`: a boot-hook installer (idempotent, writes into `/etc/rc.local`) and a two-subcommand maintenance script (`unlock`/`rebuild`). The plan ends with an actual physical reboot of the board (pre-approved during design — `/etc/rc.local` failures are non-blocking to boot, so this carries very low risk) to prove the hook actually fires unattended.

**Tech Stack:** POSIX `sh` (BusyBox ash on the board, OpenWrt 15.05.1), the project's SSH key (`provisioning/lib/keys/board_rsa`, RSA 2048 PEM — regenerated 2026-07-13, already installed on the board's `authorized_keys`), native OpenSSH `ssh`/`scp` with this board's four legacy-algorithm overrides (established pattern, see `mt7688-mcp-server/deploy.ps1`).

## Global Constraints

- Board host: `192.168.31.156` (DHCP-assigned — if unreachable, re-verify with `ping -4 mylinkit.local` before assuming something in this plan broke it; see `CLAUDE.md`).
- Every remote command in this plan uses this exact SSH invocation shape (four legacy-algorithm overrides this board's old Dropbear requires):
  ```
  ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 '<command>'
  ```
  and file transfer uses `scp` with the same flags plus `-O` (forces the legacy SCP protocol — Dropbear has no `sftp-server`):
  ```
  scp -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 -O <local> root@192.168.31.156:<remote>
  ```
- `/tmp` on the board is tmpfs and gets cleared periodically — don't let time pass between `scp`-ing a script and running it (established project caveat, `CLAUDE.md`).
- Every new script gets the standard repo header comment: `# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)`.
- If any verification step in this plan produces an unexpected result, **stop and report BLOCKED** with exactly what happened — don't improvise a workaround on live boot configuration.

---

## Task 1: Boot-hook installer script

**Files:**
- Create: `provisioning/05-squashfs-opt/06_install_boot_hook.sh`

**Interfaces:**
- Produces: the marker string `# squashfs /opt boot mount` inside the board's `/etc/rc.local` — Task 4 (dev-notes/TODO update) references this same mechanism by name; Task 3's reboot verification depends on this block actually running at boot.

- [ ] **Step 1: Write the installer script**

Create `provisioning/05-squashfs-opt/06_install_boot_hook.sh`:

```sh
#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# Idempotently installs the squashfs /opt boot-mount hook into /etc/rc.local.
# See docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md
MARKER="# squashfs /opt boot mount"
RC=/etc/rc.local

if grep -qF "$MARKER" "$RC"; then
    echo "ALREADY_INSTALLED"
    exit 0
fi

TMP=/tmp/rc.local.new
grep -v '^exit 0$' "$RC" > "$TMP"
cat >> "$TMP" <<'HOOK'
# squashfs /opt boot mount - see docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md
if ! mount | grep -q ' /opt '; then
    if [ -f /overlay/opt.sqfs ]; then
        losetup /dev/loop0 /overlay/opt.sqfs 2>/tmp/opt-sqfs-boot.log \
            && mount -t squashfs -o ro /dev/loop0 /opt 2>>/tmp/opt-sqfs-boot.log \
            || logger -t opt-sqfs "boot mount failed, /opt stays writable - see /tmp/opt-sqfs-boot.log"
    else
        logger -t opt-sqfs "/overlay/opt.sqfs missing, /opt stays writable"
    fi
fi

exit 0
HOOK

cp "$RC" "$RC.bak"
mv "$TMP" "$RC"
chmod 755 "$RC"
echo "INSTALLED"
cat "$RC"
```

- [ ] **Step 2: Deploy it to the board**

Run:
```
scp -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 -O "d:/Code/LinkItSmart7688Duo/provisioning/05-squashfs-opt/06_install_boot_hook.sh" root@192.168.31.156:/tmp/06_install_boot_hook.sh
```
Expected: file transfers with no error (scp prints a progress line, exits 0).

- [ ] **Step 3: Run it and verify the hook is installed**

Run immediately after Step 2 (don't let `/tmp` get cleared):
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 'chmod 755 /tmp/06_install_boot_hook.sh && /tmp/06_install_boot_hook.sh'
```
Expected: prints `INSTALLED` followed by the full new `/etc/rc.local` contents, ending in the hook block and a final `exit 0`.

- [ ] **Step 4: Verify idempotency — run it a second time**

Run the same command from Step 3 again.
Expected: prints only `ALREADY_INSTALLED` (no duplicate block appended). Confirm by also running:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 'grep -c "squashfs /opt boot mount" /etc/rc.local'
```
Expected: `1` (the marker line appears exactly once).

- [ ] **Step 5: Commit**

```bash
git add provisioning/05-squashfs-opt/06_install_boot_hook.sh
git commit -m "feat: add idempotent squashfs /opt boot-hook installer

Deployed and verified live: installs cleanly into /etc/rc.local,
re-running is a no-op (ALREADY_INSTALLED, marker appears exactly once)."
```

---

## Task 2: Maintenance script (`unlock`/`rebuild`) — write, deploy, and exercise it once

**Files:**
- Create: `provisioning/05-squashfs-opt/07_opt_maintenance.sh`

**Interfaces:**
- Consumes: nothing from Task 1 (independent script).
- Produces: a rebuilt `/overlay/opt.sqfs` mounted read-only at `/opt` by the end of this task — Task 3's reboot verification depends on this file existing and being current (not the stale 2026-07-12 one).

- [ ] **Step 1: Write the maintenance script**

Create `provisioning/05-squashfs-opt/07_opt_maintenance.sh`:

```sh
#!/bin/sh
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)
# unlock:  expose the real writable /opt for opkg operations.
# rebuild: rebuild /overlay/opt.sqfs from the current /opt and remount it.
# See docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md
set -e

cmd="$1"

do_unlock() {
    if mount | grep -q ' /opt '; then
        umount /opt
        echo "unmounted /opt"
    else
        echo "/opt was not mounted"
    fi
    if losetup -d /dev/loop0 2>/dev/null; then
        echo "detached /dev/loop0"
    else
        echo "/dev/loop0 was not attached"
    fi
}

do_rebuild() {
    if mount | grep -q ' /opt '; then
        echo "ERROR: /opt is still mounted read-only - run 'unlock' first" >&2
        exit 1
    fi
    E=/tmp/mksq
    rm -rf $E; mkdir -p $E
    cp /opt/lib/ld-2.27.so $E/ 2>/dev/null || cp /opt/lib/ld.so.1 $E/ld-2.27.so
    for f in /opt/lib/*.so*; do cp -a "$f" $E/ 2>/dev/null; done
    cp /opt/sbin/mksquashfs $E/
    chmod 755 $E/mksquashfs $E/ld-2.27.so
    sync
    echo "--- staged $(busybox du -sh $E | busybox cut -f1) into tmpfs, building..."
    rm -f /overlay/opt.sqfs
    $E/ld-2.27.so --library-path $E $E/mksquashfs /opt /overlay/opt.sqfs \
        -comp xz -b 131072 -noappend -quiet
    echo "mksquashfs rc=$?"
    ls -la /overlay/opt.sqfs
    rm -rf $E

    losetup /dev/loop0 /overlay/opt.sqfs
    mount -t squashfs -o ro /dev/loop0 /opt
    echo "remounted /opt from rebuilt image"
    mount | grep ' /opt '
}

case "$cmd" in
    unlock)  do_unlock ;;
    rebuild) do_rebuild ;;
    *)
        echo "usage: $0 {unlock|rebuild}" >&2
        exit 1
        ;;
esac
```

- [ ] **Step 2: Deploy it to the board**

Run:
```
scp -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 -O "d:/Code/LinkItSmart7688Duo/provisioning/05-squashfs-opt/07_opt_maintenance.sh" root@192.168.31.156:/tmp/07_opt_maintenance.sh
```
Expected: transfers with no error.

- [ ] **Step 3: Run `unlock` — should be a harmless no-op right now**

Nothing is mounted yet at this point in the plan (confirmed live 2026-07-14 during design). Run immediately after Step 2:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 'chmod 755 /tmp/07_opt_maintenance.sh && /tmp/07_opt_maintenance.sh unlock'
```
Expected: prints `/opt was not mounted` and `/dev/loop0 was not attached`. Exit code 0.

- [ ] **Step 4: Run `rebuild` — first real exercise of the mechanism**

Run:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 '/tmp/07_opt_maintenance.sh rebuild'
```
Expected:
- `mksquashfs rc=0`
- an `ls -la` line showing `/overlay/opt.sqfs` with a fresh timestamp and non-zero size (a few MB, not necessarily the same 28.7MB as the stale 07-12 image — size depends on current `/opt` contents)
- `remounted /opt from rebuilt image`
- a `mount | grep ' /opt '` line showing something like `/dev/loop0 on /opt type squashfs (ro,relatime)`

If `mksquashfs rc=` is non-zero, or the final `mount | grep` line is empty, **stop and report BLOCKED** with the full output — don't retry blindly, the most likely cause is a changed `/opt/lib`/`/opt/sbin/mksquashfs` layout since 2026-07-12 that the staging `cp` commands need to account for.

- [ ] **Step 5: Confirm the mount is actually read-only**

Run:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 'touch /opt/write-test-should-fail 2>&1; echo "exit=$?"'
```
Expected: an error mentioning `Read-only file system`, and `exit=1` (or similar nonzero). A `exit=0` here means the mount didn't actually take and rebuild silently no-op'd — treat that as BLOCKED, not a pass.

- [ ] **Step 6: Commit**

```bash
git add provisioning/05-squashfs-opt/07_opt_maintenance.sh
git commit -m "feat: add squashfs /opt unlock/rebuild maintenance script

Exercised live: unlock correctly no-ops when nothing is mounted,
rebuild produces a fresh /overlay/opt.sqfs and remounts it read-only
at /opt (write attempt confirmed EROFS)."
```

---

## Task 3: Reboot verification

**Files:** None — this task is pure live verification, no repo changes.

**Interfaces:**
- Consumes: the boot hook from Task 1 (must already be installed in `/etc/rc.local`) and the fresh image from Task 2 (must already be built and present at `/overlay/opt.sqfs`).

- [ ] **Step 1: Unmount so the reboot's mount is unambiguously the boot hook's doing, not a leftover**

`/tmp` on the board is tmpfs and gets cleared periodically (project-standing caveat) — don't assume Task 2's copy at `/tmp/07_opt_maintenance.sh` is still there if any real time has passed since Task 2 finished. Re-deploy it fresh before using it:
```
scp -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 -O "d:/Code/LinkItSmart7688Duo/provisioning/05-squashfs-opt/07_opt_maintenance.sh" root@192.168.31.156:/tmp/07_opt_maintenance.sh
```
Then, immediately after:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 'chmod 755 /tmp/07_opt_maintenance.sh && /tmp/07_opt_maintenance.sh unlock'
```
Expected: `unmounted /opt` and `detached /dev/loop0` this time (both were attached from Task 2).

This step is about eliminating ambiguity in the verification, not undoing Task 2's result — the image built in Task 2 stays on disk at `/overlay/opt.sqfs`; only the current mount is torn down so Step 3's post-reboot check can't be a false positive from a mount that was already up before the reboot.

- [ ] **Step 2: Reboot the board**

Run:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 'reboot'
```
Expected: the SSH connection drops (either a clean disconnect or the command hangs briefly then the socket closes) — this is normal, the board is going down.

- [ ] **Step 3: Wait for the board to come back, then reconnect**

The board takes roughly 30-60 seconds to fully reboot (OpenWrt on this hardware, SD extroot). Poll rather than guessing a fixed sleep:
```
ping -4 -n 1 192.168.31.156
```
Retry every ~10 seconds until it responds, then confirm SSH specifically (not just ICMP) is back:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 -o ConnectTimeout=5 root@192.168.31.156 'echo BACK; uptime'
```
Expected: `BACK` followed by an `uptime` line showing a small uptime (minutes, not the pre-reboot value) — confirms this is genuinely a fresh boot, not a stale cached connection.

If the board doesn't come back within ~5 minutes of polling, **stop and report BLOCKED** — do not attempt any further remote recovery action; this is the one scenario the design's risk analysis didn't cover (rc.local itself can't hang boot, but this would point at something else going wrong), and it needs the user's physical access.

- [ ] **Step 4: Verify the boot hook actually mounted `/opt` unattended**

Run:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 'mount | grep " /opt "; echo "---"; touch /opt/write-test-should-fail 2>&1; echo "exit=$?"'
```
Expected: a `mount` line showing `/dev/loop0 on /opt type squashfs (ro,...)`, then the same `Read-only file system` / nonzero exit pattern as Task 2 Step 5 — this time produced entirely by the unattended boot hook, not a manual script invocation.

- [ ] **Step 5: Confirm the mount is actually usable, not just present**

Run:
```
ssh -i "d:/Code/LinkItSmart7688Duo/provisioning/lib/keys/board_rsa" -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.31.156 '
sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
/opt/lib/ld.so.1 --library-path /opt/lib /opt/bin/grep --version | head -1
sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null
/opt/lib/ld.so.1 --library-path /opt/lib /opt/bin/python3.13 --version
'
```
Expected: both commands print a normal version string (`grep (GNU grep) ...`, `Python 3.13.x`) with no `Illegal instruction`/`Segmentation fault`/`Bus error` — the exact cold-cache probe pattern from `04_mount_and_probe.sh`, now running against the boot-hook-created mount instead of a manually-triggered one.

If either command crashes or hangs, **stop and report BLOCKED** — this would mean the boot-hook mount is present but not actually providing the crash workaround, which defeats the entire point of this plan; don't proceed to lifting the dev-notes ban.

- [ ] **Step 6: No commit for this task** — it's pure verification, no files changed. Proceed directly to Task 4.

---

## Task 4: Lift the dev-notes.html ban and close out TODO.md

**Files:**
- Modify: `dev-notes.html`
- Modify: `TODO.md`

**Interfaces:** None — this is documentation only, gated on Task 3 having passed completely.

- [ ] **Step 1: Read the current ban text**

Run (this repo, not the board):
Read `dev-notes.html` around the text found during design — search for `不要把 <code>/opt</code>` to locate the exact paragraph (was lines ~740-746 as of this plan's writing, but re-locate it fresh rather than trusting a hardcoded line number, since the file may have changed).

- [ ] **Step 2: Update the ban paragraph**

Replace the paragraph so it records the resolution instead of the still-open caveat. The existing text ends with:
> `2026-07-12 起這條禁令有了出口：squashfs 繞道已實驗證實（見上節，70/70），等開機自動掛載上線（TODO.md 第一項）之後，這條界線就可以正式解除。`

Change it to record that the automation has landed and the ban is lifted, referencing this plan's design doc, e.g.:
> `2026-07-14 起這條禁令已解除：squashfs /opt 開機自動掛載已上線並實機驗證通過（見 docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md 與 provisioning/05-squashfs-opt/06_install_boot_hook.sh），/opt 底下的服務現在可以安全放進開機自動啟動。`

Keep the preceding sentences (the `uv`/dependency-resolution context) unchanged — only the final sentence about the ban is stale.

- [ ] **Step 3: Update TODO.md**

Change the top-priority section's three checkboxes from `- [ ]` to `- [x]`, and add a result note beneath them in the same style as the existing "✅ 已完成:squashfs 實驗" section below it (bold **結果**, brief description, pointer to the design/plan docs). Move or relabel the section header to indicate completion, consistent with how the 2026-07-12 experiment section is already written.

- [ ] **Step 4: Commit**

```bash
git add dev-notes.html TODO.md
git commit -m "docs: close out squashfs /opt boot-mount TODO, lift autostart ban

Verified live 2026-07-14 across a real reboot: /opt auto-mounts
read-only from /overlay/opt.sqfs via /etc/rc.local with no manual
step, and stays crash-free under the same cold-cache probe that
originally found the bug. Entware services can now be safely added
to boot autostart."
```

---

## Self-Review Notes

- **Spec coverage:** Design's three goals map 1:1 to Tasks 1 (auto-mount), 2+3 (maintenance workflow + its live proof), and 4 (lift the ban) — all covered. The "non-goals" (no `rc.unslung` wiring, no preinit changes, no writable-subdir support) are correctly absent from every task.
- **Placeholder scan:** No TBD/TODO markers; every script step has complete, runnable code; every verification step has a concrete expected output, not "add appropriate checks."
- **Type/name consistency:** `06_install_boot_hook.sh` and `07_opt_maintenance.sh` are the only two new artifacts; their names and the `unlock`/`rebuild` subcommand names are used identically across Tasks 1-3 wherever referenced.
