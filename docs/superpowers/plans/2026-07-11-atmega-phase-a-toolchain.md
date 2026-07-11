<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# ATmega Bridge Phase A: Toolchain Bring-Up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove `arduino-cli` can compile a sketch and flash it onto this board's ATmega32U4 over the network (no USB), verified by a human watching an LED actually blink — per `docs/superpowers/specs/2026-07-11-atmega-phase-a-toolchain.md`.

**Architecture:** This is infrastructure bring-up, not application code — there is no TDD cycle here (no unit-testable logic, the only "test" is a physical LED). Three tasks: (1) install `arduino-cli` and discover the exact board index URL + FQBN for this board (both genuinely unknown right now — this plan deliberately does NOT hardcode a guessed URL or FQBN, since getting either wrong would silently produce a broken setup; the task requires looking them up from real, current sources before using them), (2) write and compile a minimal onboard-LED-blink sketch, (3) flash it to the real board via SSH + the board's pre-installed `run-avrdude` ISP mechanism and get visual confirmation.

**Tech Stack:** `arduino-cli` (Windows), the board's stock `avrdude`/`run-avrdude` (already installed on-device, confirmed during design), the existing project SSH key (`provisioning/lib/keys/board_rsa`, same 4-flag pattern used throughout this project).

---

## Task 1: Install `arduino-cli` and discover the board index URL + FQBN

**Files:** None — this task doesn't touch the repo at all, it's local tool installation plus research.

Two real unknowns block everything else in this plan: (a) the current, correct way to install `arduino-cli` on Windows, and (b) the board manager index URL for "LinkIt Smart 7688 Duo" (which in turn determines the FQBN). Guessing either risks silently building for the wrong target or failing later in a confusing way — both must be looked up from real, current sources, not assumed from memory.

- [ ] **Step 1: Install `arduino-cli`**

Look up the current official installation instructions for `arduino-cli` on Windows — check `https://arduino.github.io/arduino-cli/latest/installation/` (the official docs site) via WebFetch, or the releases page at `https://github.com/arduino/arduino-cli/releases` for the latest Windows zip. Follow whatever the official docs actually say today (this changes over time; don't rely on a remembered command). Install it somewhere on `PATH`.

Verify:

Run: `arduino-cli version`
Expected: prints a version string, e.g. `arduino-cli  Version: 1.x.x ...` — any successful version output confirms the binary is installed and runnable.

- [ ] **Step 2: Initialize config and find the board index URL for this board**

Run: `arduino-cli config init`
Expected: creates a default config file, prints its path.

Look up the board manager URL needed for the "LinkIt Smart 7688 Duo" board specifically — check Seeed Studio's wiki page for this board (Seeed is the vendor selling it) and/or MediaTek Labs' developer site for their Arduino board package index, via WebFetch/WebSearch. Do not guess a URL from memory — confirm it against a real, current source, since this project has already been burned once this session by trusting an assumption that turned out to be wrong without checking (see the design spec's GPIO pin 3/38 finding for an example of why "verify against the real source" matters here).

Once you have a real URL, add it:

Run: `arduino-cli config add board_manager.additional_urls <the-url-you-found>`
Expected: confirms the URL was added (or already present).

Run: `arduino-cli core update-index`
Expected: downloads package indexes, including the one just added, with no errors for that specific index (errors for unrelated default indexes, if any, aren't your concern).

- [ ] **Step 3: Install the board core and confirm the FQBN**

Run: `arduino-cli core list`
Expected: shows an installed-cores list; find the core name matching MediaTek/LinkIt (exact name depends on what Step 2 found — record it).

If not yet installed, run: `arduino-cli core install <core-name-from-above>`
Expected: downloads and installs the core successfully.

Run: `arduino-cli board listall | grep -i 7688`
Expected: at least one line showing the board name and its FQBN (format like `vendor:arch:boardname`). **Record this exact FQBN string — every later task needs it verbatim.**

- [ ] **Step 4: Report findings**

Before moving to Task 2, write down (for the next task, which will need this exact information):
- The exact `arduino-cli` version installed.
- The exact board manager URL that worked.
- The exact core name installed.
- The exact FQBN string found for this board.

If any step failed or produced an unexpected result (e.g. no board index could be found, or `board listall` shows nothing matching "7688"), **stop and report BLOCKED with what you found** — don't guess a fallback FQBN and proceed, since everything downstream depends on this being right.

---

## Task 2: Write and compile the onboard-LED-blink sketch

**Files:**
- Create: a new sketch directory and `.ino` file — exact path decided in this task (see Step 1), since this is the first ATmega-side code in the repo and `CLAUDE.md`'s "Code lives in" line for the ATmega side is still a TODO waiting to be filled in with a real, now-existing path (matching this repo's own convention of not documenting a path until code actually lives there).

- [ ] **Step 1: Create the sketch**

Arduino requires a sketch's `.ino` file to be inside a directory with the exact same name as the file (minus extension). Use `atmega-firmware/blink-test/blink-test.ino` (repo root: `d:\LinkItSmart7688Duo`) — this establishes `atmega-firmware/` as the top-level directory for ATmega-side code going forward, parallel to `mt7688-mcp-server/` for the MT7688 side.

Create `d:\LinkItSmart7688Duo\atmega-firmware\blink-test\blink-test.ino`:

```cpp
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

// Phase A toolchain bring-up test: blinks the ATmega's own onboard LED
// (LED_BUILTIN). Does not touch Serial1 (the bridge UART) or communicate
// with the MT7688 side at all - this sketch exists purely to prove the
// compile-and-flash path works, per docs/superpowers/specs/
// 2026-07-11-atmega-phase-a-toolchain.md.

void setup() {
  pinMode(LED_BUILTIN, OUTPUT);
}

void loop() {
  digitalWrite(LED_BUILTIN, HIGH);
  delay(500);
  digitalWrite(LED_BUILTIN, LOW);
  delay(500);
}
```

- [ ] **Step 2: Compile it**

Using the exact FQBN recorded in Task 1:

Run (from `d:\LinkItSmart7688Duo`): `arduino-cli compile --fqbn <fqbn-from-task-1> --output-dir atmega-firmware/blink-test/build atmega-firmware/blink-test`

Expected: compiles successfully, prints something like `Sketch uses X bytes...`, and produces a `.hex` file under `atmega-firmware/blink-test/build/` (exact filename will be something like `blink-test.ino.hex` — confirm the exact name, the next task needs it).

If compilation fails, **stop and report BLOCKED** with the exact error — don't work around a compile error by guessing at the FQBN or board settings; report what happened so the FQBN/core choice from Task 1 can be re-checked.

- [ ] **Step 3: Commit the sketch**

The `build/` output directory should not be committed (it's a build artifact, regenerated by `arduino-cli compile`). Add a `.gitignore` entry for it:

Create `d:\LinkItSmart7688Duo\atmega-firmware\.gitignore`:
```
blink-test/build/
```

```bash
git add atmega-firmware/blink-test/blink-test.ino atmega-firmware/.gitignore
git commit -m "feat: add ATmega toolchain bring-up test sketch (LED blink)"
```

---

## Task 3: Flash over the network and get visual confirmation

**Files:** None (deployment/verification only) — plus a `CLAUDE.md` update in the final step.

This task requires live coordination with the user (asking them to look at the board and confirm the LED is blinking) and uses board SSH credentials — per this project's established practice, this task is executed directly by the controller (not delegated to a subagent), the same way the final "deploy and live-verify" task of every prior MCP-tool plan was handled personally rather than delegated.

- [ ] **Step 1: Copy the `.hex` to the board**

Using the exact `.hex` filename confirmed in Task 2, and the same SSH key + 4-legacy-flag pattern used by `mt7688-mcp-server/deploy.ps1` throughout this project:

```powershell
scp -O -i provisioning/lib/keys/board_rsa -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 atmega-firmware/blink-test/build/blink-test.ino.hex root@192.168.50.216:/tmp/blink-test.hex
```

(Adjust the local filename if Task 2 found a different exact name, and confirm the board's current IP/hostname is still `192.168.50.216` / `mylinkit.local` before running this — it may have changed since this plan was written.)

Expected: file transfer completes with no errors.

- [ ] **Step 2: Flash via the board's stock ISP mechanism**

```powershell
ssh -i provisioning/lib/keys/board_rsa -o KexAlgorithms=+diffie-hellman-group14-sha1 -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa -o MACs=+hmac-sha1 root@192.168.50.216 "run-avrdude /tmp/blink-test.hex"
```

Expected: `avrdude` output showing it wrote and verified flash memory successfully (look for something like `avrdude: X bytes of flash written` and `avrdude: X bytes of flash verified` with no error lines). This uses the ISP-over-GPIO mechanism documented in the design spec (`reset=GPIO3, sck=GPIO15, mosi=GPIO16, miso=GPIO17`) — no bootloader, no serial protocol.

If this fails (e.g. `avrdude` reports it can't find/sync with the target device), **stop and report BLOCKED** with the exact error. Per the design spec, USB is available as a manual fallback if the user wants to try wired flashing instead, but that's the user's call to make, not something to silently fall back to.

- [ ] **Step 3: Get visual confirmation**

Ask the user to look at the board and confirm the LED is blinking (roughly once per second, per the 500ms on/500ms off delay in the sketch). This is the actual pass/fail signal for this entire plan — `avrdude` reporting success only means the flash write succeeded, not that the resulting firmware does what's intended.

- [ ] **Step 4: Update `CLAUDE.md`**

Now that `atmega-firmware/` exists as a real path, fill in the ATmega side's "Code lives in" TODO. In `CLAUDE.md`'s board architecture table, change the ATmega32U4 side's "Code lives in" cell from `(TODO: e.g. `firmware/`, `sketch/`)` to `` `atmega-firmware/` ``.

Also fill in the FQBN TODO in the "Working on the ATmega32U4 (Arduino) side" section — replace `TODO: record the exact FQBN for this board once confirmed via `arduino-cli board listall | grep -i 7688`` with the actual FQBN confirmed in Task 1, and update the compile/upload example commands to use it.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: record ATmega FQBN and code location after toolchain bring-up"
```

---

## Plan self-review notes

This plan intentionally does not hardcode the board manager URL or FQBN — both are genuinely unknown as of when this plan was written, and Task 1 exists specifically to discover them safely (via real lookups, not memory) before anything downstream depends on them. This is a deliberate deviation from this project's usual "complete code in every step" plans (which apply to Go application code with well-understood APIs); infrastructure bring-up against an external toolchain whose exact configuration isn't yet known doesn't have that luxury, and guessing would be worse than looking it up. Task 2 and Task 3 are fully concrete once Task 1's findings are known.
