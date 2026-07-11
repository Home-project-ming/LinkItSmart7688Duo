<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# ATmega Bridge Phase C: ADC Firmware Implementation Plan

Status: implemented, verified live on real hardware 2026-07-11

## Live verification (2026-07-11)

All three tasks executed and verified for real:

- Task 1 (Firmata library investigation): hit and fixed an unrelated environment blocker (`arduino-cli lib install` failing under this machine's OneDrive-redirected Documents folder — fixed via `directories.user` in `arduino-cli.yaml`). All 6 planned findings confirmed against the real installed library source (independently re-verified by a second subagent reading the same files): `begin(Stream&)` exists, `attach()`/`sendAnalog()` signatures match, `REPORT_ANALOG`/`ANALOG_MESSAGE` are exactly `0xC0`/`0xE0`, and `NUM_ANALOG_INPUTS` turned out to be a non-issue — Firmata's `Boards.h` hardcodes `TOTAL_ANALOG_PINS 12` for `__AVR_ATmega32U4__` independent of the board's own (incomplete) `pins_arduino.h`.
- Task 2 (sketch): compiled clean on the first try (8492/28672 bytes, 29%). Code quality review caught one real issue — the header comment claimed only two Firmata message types are ever sent, but `Firmata.begin(Serial1)` internally fires an unsolicited one-time `REPORT_VERSION` (`0xF9`) at boot via the library's own `printVersion()`. Fixed as a comment-only correction (`762da43`) so Phase D isn't surprised by it later; no functional code changed.
- Task 3 (flash + verify): flashed successfully via `run-avrdude` (8492 bytes written and verified). **The bridge UART device node was confirmed as `/dev/ttyS0`** — not by elimination, but directly from `dmesg`'s pinmux allocation log (`uart0` group → pins 12/13 → `10000c00.uartlite` → `ttyS0`), now recorded in `CLAUDE.md`.
  - **Deviation from the plan's Step 3-5:** the user explicitly declined the physical A0-to-3.3V/GND jumper wiring ("我不想動硬體，因為還要找ping腳很麻煩") and pointed out a floating analog pin should still produce a real (if noisy) reading. Verification was adapted accordingly: `REPORT_ANALOG` was enabled for channel 0 on `/dev/ttyS0` with A0 left floating (default state, no jumper), and the raw bytes read back with `hexdump -C` (busybox on this board has neither `od` nor `timeout`, unlike what the plan assumed — used `hexdump -C` and `stty min/time` bounds instead). Result: a clean stream of 20 well-formed `ANALOG_MESSAGE` frames (`e0 <lsb> <msb>`), values jittering between 0-21 — consistent with real ADC noise on a floating pin, not a stuck/fake value. Disabling reporting (`REPORT_ANALOG` off) was then confirmed to stop the stream entirely (a follow-up read produced zero bytes). This is weaker than a deterministic 3.3V/GND reference test would have been (no proof the *value* is quantitatively correct, only that the protocol round-trip and enable/disable logic are), but was judged sufficient given the user's explicit preference — noted here honestly rather than silently upgraded to "fully verified."

**Net result:** the firmware correctly implements the Phase B wire protocol end-to-end on real hardware — confirmed by directly reading raw bytes off the bridge UART, not just by a successful compile/flash. Phase D (MT7688-side Go decoder + MCP tool) is not built and is a separate future plan.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement ATmega-side firmware that speaks the ADC-only Firmata subset (`REPORT_ANALOG`/`ANALOG_MESSAGE`, channels 0-11, 57600 baud) designed in `docs/superpowers/specs/2026-07-11-atmega-phase-b-wire-protocol.md`, using the standard Firmata Arduino library, flashed to the real board and physically verified.

**Architecture:** Three tasks. (1) Install the Firmata library and investigate its real API against this board — genuinely unknown, must be verified against the actually-installed source, not guessed, mirroring how Phase A had to discover the real FQBN/board-manager URL rather than assume one. (2) Write and compile the custom ADC-report sketch using Task 1's confirmed findings. (3) Flash over the network via the already-proven ISP mechanism and physically verify a real analog reading round-trips correctly over the raw bridge UART — handled personally by the controller (not a subagent), matching how every prior hardware deploy/verify step in this project (Phase A Task 3, and the final step of every MCP-tool plan) was handled.

**Tech Stack:** `arduino-cli`, the Firmata Arduino library (exact version confirmed in Task 1), FQBN `LinkIt:avr:smart7688`, the existing SSH key + `run-avrdude` ISP flashing mechanism from Phase A.

---

## Context carried in from Phase B and Phase A (don't re-derive)

- FQBN: `LinkIt:avr:smart7688`. Core manually installed per Phase A's dead-URL workaround (`Arduino15/packages/LinkIt/hardware/avr/0.1.7/`).
- This board's `pins_arduino.h` (`.../0.1.7/variants/smart7688/pins_arduino.h`) confirms 12 real analog channels, A0-A11 (channels 0-5 dedicated, 6-11 shared with digital D4/D6/D8/D9/D10/D12), via its `analog_pin_to_channel_PGM[12]` array.
- **This board's `pins_arduino.h` does NOT define `NUM_ANALOG_INPUTS`** (confirmed absent via a full-package grep; the standard Arduino `leonardo` variant does define it, `= 12`, so this is a real gap specific to this vendor package, not normal). Don't assume any macro derived from it (e.g. Firmata's `TOTAL_ANALOG_PINS`, if it turns out to depend on `NUM_ANALOG_INPUTS`) is available — Task 1 checks this for real; Task 2's code sidesteps the question entirely by defining its own constant (see Task 2).
- Confirmed in `wiring_analog.c` (`Arduino15/packages/arduino/hardware/avr/1.8.8/cores/arduino/wiring_analog.c`): `analogRead()` has an explicit `#if defined(__AVR_ATmega32U4__)` branch that subtracts 18 from the pin argument when `pin >= 18`, before calling `analogPinToChannel()`. This means `analogRead(0)` (raw 0-based channel) and `analogRead(18)` (the `A0` macro) both resolve to the same, correct ADC channel on this exact board — verified by reading the real core source, not assumed. The sketch below uses raw 0-based channel numbers (matching Firmata's own channel numbering), which is confirmed safe.
- Bridge UART is `Serial1` on the ATmega side (hardware TXD1/RXD1, physical pins D0/D1 per this board's pin-out comments — **not** D12/D13; those are ISP pins). Never write debug output to `Serial1` — it's reserved for bridge traffic (established rule, `CLAUDE.md`).
- `atmega-firmware/blink-test/blink-test.ino` is the only prior ATmega sketch in this repo; `atmega-firmware/.gitignore` already exists with one entry (`blink-test/build/`).
- Board host for live steps: `192.168.50.216`, SSH key `provisioning/lib/keys/board_rsa`, 4 legacy algorithm flags (`KexAlgorithms=+diffie-hellman-group14-sha1`, `HostKeyAlgorithms=+ssh-rsa`, `PubkeyAcceptedAlgorithms=+ssh-rsa`, `MACs=+hmac-sha1`) — same pattern used throughout this project (`mt7688-mcp-server/deploy.ps1`).
- Flashing mechanism: `scp` the `.hex` to `/tmp/` on the board, then `ssh ... "run-avrdude /tmp/<name>.hex"`. `/tmp` on the board is `tmpfs` and gets cleared periodically — `scp` immediately before running `run-avrdude`, no delay in between (Phase A's tmpfs finding).

## Task 1: Install the Firmata library and confirm its real API for this board

**Files:** None — investigation only, same nature as Phase A's Task 1 (no repo changes, local toolchain + research).

Firmata's exact API surface (whether `begin()` can target `Serial1` instead of the default `Serial`, the exact `REPORT_ANALOG`/`ANALOG_MESSAGE` constant values, whether anything depends on the `NUM_ANALOG_INPUTS` macro this board's package is missing) is genuinely unknown until the library is actually installed and its real source read. Guessing any of this risks the same class of silent-wrong-assumption that the missing `LED_BUILTIN` caused in Phase A — this task exists specifically to avoid that.

- [ ] **Step 1: Find and install the library**

Run: `arduino-cli lib search firmata`
Expected: a result named exactly `Firmata` (published by the Firmata Development Team). Record the exact name.

Run: `arduino-cli lib install Firmata` (adjust to the exact name found above if it differs)
Expected: downloads and installs successfully.

- [ ] **Step 2: Locate the installed library and record its version**

Run: `arduino-cli config dump` and find the user directory (the `directories.user` field).
The library will be under `<user-dir>/libraries/Firmata/`. Confirm this path exists.

Read `<user-dir>/libraries/Firmata/library.properties` and record the `version=` line.

- [ ] **Step 3: Smoke-test compile the library's own bundled example**

The library ships `examples/StandardFirmata/StandardFirmata.ino`. Compile it against this board's FQBN as a basic compatibility check, before writing any new code:

Run: `arduino-cli compile --fqbn LinkIt:avr:smart7688 --output-dir <a temp dir> "<user-dir>/libraries/Firmata/examples/StandardFirmata"`
Expected: compiles successfully.

If this fails, **stop and report BLOCKED** with the exact compiler error. Don't guess a library-version pin or workaround — this needs to be re-assessed (e.g. checking whether a specific Firmata version is needed for this old AVR core) before Task 2 can proceed on solid ground.

- [ ] **Step 4: Read `Firmata.h` and record the real API**

Read `<user-dir>/libraries/Firmata/Firmata.h` (and `Firmata.cpp` if a signature isn't clear from the header alone). Record, in writing, for use in Task 2:

1. Every `begin(...)` overload on `FirmataClass` — specifically, does one accept a `Stream &` (e.g. usable as `Firmata.begin(Serial1)`)? This is a hard requirement: Firmata traffic must go over `Serial1` (the bridge UART), never `Serial` (the USB CDC debug console) — if no such overload exists, **stop and report BLOCKED**, this is an architecture problem, not something to patch around inside this task.
2. The exact signature of `attach(byte type, callback)` and which callback function-pointer type applies to `REPORT_ANALOG` (a host→device message, 2 arguments: pin/channel and value).
3. The exact signature of `sendAnalog(byte pin, int value)` (or whatever it's actually called — record the real name).
4. The actual numeric values of the `REPORT_ANALOG` and `ANALOG_MESSAGE` constants. Confirm they match `0xC0`-based / `0xE0`-based as documented in the Phase B spec. If they differ, note the real values — Task 2 must use the real ones.

- [ ] **Step 5: Check whether Firmata.h depends on `NUM_ANALOG_INPUTS`**

Search the library source (`Firmata.h`, `Firmata.cpp`, `Boards.h` if present in this library version) for `NUM_ANALOG_INPUTS` and `TOTAL_ANALOG_PINS`. Record whether either is referenced, and if so, whether it's hardcoded per-chip (e.g. inside an `#elif defined(__AVR_ATmega32U4__)` block) or expected to come from the board's own `pins_arduino.h` (which this board's package does not define). This determines whether including `Firmata.h` alone will compile cleanly on this board or needs a `#define NUM_ANALOG_INPUTS 12` before the `#include` — record the answer either way; Task 2's own sketch code doesn't rely on this macro for its own loop bound regardless (see Task 2), but compilation could still fail here if `Firmata.h` itself needs it and doesn't get it.

- [ ] **Step 6: Report findings**

Summarize all of the above (library name + version + install path, `begin()`'s real Stream-argument support, `attach()`/callback signature, `sendAnalog()`'s real name and signature, the real `REPORT_ANALOG`/`ANALOG_MESSAGE` values, and the `NUM_ANALOG_INPUTS` dependency finding) before starting Task 2.

---

## Task 2: Write and compile the ADC-report sketch

**Files:**
- Create: `atmega-firmware/adc-bridge/adc-bridge.ino`
- Modify: `atmega-firmware/.gitignore`

- [ ] **Step 1: Create the sketch**

Use the API confirmed in Task 1. The code below is the expected shape based on Firmata's well-established public API — if Task 1 found different real names/signatures, use those instead of guessing new ones; if the code below fails to compile for a reason Task 1's findings don't explain, **stop and report BLOCKED** rather than trial-and-error guessing a fix.

Create `d:\LinkItSmart7688Duo\atmega-firmware\adc-bridge\adc-bridge.ino`:

```cpp
// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

// ATmega bridge Phase C: ADC-read-only Firmata firmware. Implements the
// message subset from docs/superpowers/specs/
// 2026-07-11-atmega-phase-b-wire-protocol.md - REPORT_ANALOG (host enables
// or disables periodic reporting for one of this board's 12 analog
// channels, 0-11 = A0-A11) and ANALOG_MESSAGE (this firmware's periodic
// reply, a 14-bit value in Firmata's standard 2x7-bit encoding). No
// digital I/O, no SysEx - the bridge protocol is scoped to ADC reads only
// for v1.
//
// ANALOG_CHANNEL_COUNT is defined locally (not taken from a
// NUM_ANALOG_INPUTS/TOTAL_ANALOG_PINS macro) because this board's vendor
// pins_arduino.h does not define NUM_ANALOG_INPUTS at all (confirmed by
// grepping the whole installed LinkIt:avr:0.1.7 package - the standard
// Arduino "leonardo" variant does define it, this board's package is
// missing it, the same class of gap as the missing LED_BUILTIN found in
// Phase A). 12 is confirmed directly from this board's real
// analog_pin_to_channel_PGM array (12 entries, A0-A11).
//
// Runs entirely over Serial1 (the MT7688 bridge UART, per CLAUDE.md) at
// 57600 baud, the Firmata library's own default. Never touches Serial
// (the USB CDC debug console) - mixing the two would corrupt bridge
// traffic, the same rule already established for every other ATmega-side
// sketch in this project.

#include <Firmata.h>

#define ANALOG_CHANNEL_COUNT 12
#define SAMPLING_INTERVAL_MS 20

uint16_t analogChannelsToReport = 0;
unsigned long previousSampleMillis = 0;

void reportAnalogCallback(byte channel, int value)
{
  if (channel >= ANALOG_CHANNEL_COUNT) return;
  if (value == 0) {
    analogChannelsToReport &= ~((uint16_t)1 << channel);
  } else {
    analogChannelsToReport |= ((uint16_t)1 << channel);
  }
}

void setup()
{
  Serial1.begin(57600);
  Firmata.begin(Serial1);
  Firmata.attach(REPORT_ANALOG, reportAnalogCallback);
}

void loop()
{
  while (Firmata.available()) {
    Firmata.processInput();
  }

  unsigned long currentMillis = millis();
  if (currentMillis - previousSampleMillis >= SAMPLING_INTERVAL_MS) {
    previousSampleMillis = currentMillis;
    for (byte channel = 0; channel < ANALOG_CHANNEL_COUNT; channel++) {
      if (analogChannelsToReport & ((uint16_t)1 << channel)) {
        Firmata.sendAnalog(channel, analogRead(channel));
      }
    }
  }
}
```

- [ ] **Step 2: Compile it**

Run: `arduino-cli compile --fqbn LinkIt:avr:smart7688 --output-dir atmega-firmware/adc-bridge/build atmega-firmware/adc-bridge`
Expected: compiles successfully, produces a `.hex` file under `atmega-firmware/adc-bridge/build/` (record the exact filename — the next task needs it).

If compilation fails, use Task 1's recorded findings to correct any mismatched name or signature. If the failure isn't explained by anything Task 1 found, **stop and report BLOCKED** with the exact error.

- [ ] **Step 3: Extend `.gitignore` and commit**

Read the existing `d:\LinkItSmart7688Duo\atmega-firmware\.gitignore` first (it currently has one entry, `blink-test/build/`), then update it to:

```
# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

blink-test/build/
adc-bridge/build/
```

```bash
git add atmega-firmware/adc-bridge/adc-bridge.ino atmega-firmware/.gitignore
git commit -m "feat: add ATmega Firmata ADC-report firmware (bridge Phase C)"
```

---

## Task 3: Flash over the network and physically verify a real ADC reading

**Files:** None (deployment/verification), plus a `CLAUDE.md` update in the final step.

This task requires live board SSH access, physical wiring changes on the real board, and coordinating with the user to make those changes — per this project's established practice, it's executed directly by the controller, not delegated to a subagent (same as Phase A's Task 3 and the final deploy/verify step of every prior MCP-tool plan).

- [ ] **Step 1: Copy the `.hex` to the board and flash**

Using the exact `.hex` filename from Task 2:

```powershell
$BoardHost = '192.168.50.216'
$KeyFile   = 'd:\LinkItSmart7688Duo\provisioning\lib\keys\board_rsa'
$sshOpts = @(
    '-i', $KeyFile,
    '-o', 'KexAlgorithms=+diffie-hellman-group14-sha1',
    '-o', 'HostKeyAlgorithms=+ssh-rsa',
    '-o', 'PubkeyAcceptedAlgorithms=+ssh-rsa',
    '-o', 'MACs=+hmac-sha1'
)
scp -O @sshOpts atmega-firmware/adc-bridge/build/adc-bridge.ino.hex "root@${BoardHost}:/tmp/adc-bridge.hex"
& ssh @sshOpts "root@${BoardHost}" "run-avrdude /tmp/adc-bridge.hex"
```

`scp` immediately before running `run-avrdude` — no delay, per the Phase A tmpfs finding. Expected: `avrdude` reports bytes written and verified, no errors.

If this fails, **stop and report BLOCKED** with the exact error (same guidance as Phase A: USB is a known manual fallback, but that's the user's call, not a silent workaround).

- [ ] **Step 2: Identify the bridge UART's device node on the MT7688 side**

`CLAUDE.md` has flagged this as unverified since the project's original architecture notes were written. Find it now:

```powershell
& ssh @sshOpts "root@${BoardHost}" "dmesg | grep -i tty; ls /dev/ttyS*"
```

`uart_transact` (the existing MCP tool) is already confirmed fixed to `/dev/ttyS1`, which is pins 45/46 — a **different** physical UART, not the ATmega bridge (pins 12/13). Identify which other `ttyS*` node corresponds to pins 12/13 (cross-check against `dmesg` boot messages naming the UART instances, or the board's device tree/config if `dmesg` alone isn't conclusive). Record the exact node — the next step needs it.

- [ ] **Step 3: Ask the user to wire a known analog reference onto A0**

Ask the user to connect a jumper wire from the ATmega's **A0** pin to **3.3V** (a clear, deterministic high reading — expect a value near the ATmega's 10-bit ADC max, 1023) — the ATmega side's own 3.3V rail, not the MT7688 side's. Wait for confirmation before continuing.

- [ ] **Step 4: Send `REPORT_ANALOG` and read back `ANALOG_MESSAGE` over the raw bridge UART**

Using the device node found in Step 2 (substitute `<NODE>` below):

```powershell
& ssh @sshOpts "root@${BoardHost}" "stty -F /dev/<NODE> 57600 raw -echo; printf '\xc0\x01' > /dev/<NODE>; timeout 2 od -An -tx1 -N 12 /dev/<NODE>; printf '\xc0\x00' > /dev/<NODE>"
```

(`od -An -tx1` prints raw hex bytes — adjust to whatever hex-dump tool is actually available in this board's busybox if `od` isn't present; confirm with `which od` first if needed.)

Expected: the captured bytes start with `e0` (`ANALOG_MESSAGE` for channel 0), followed by two more bytes whose 14-bit combination (`byte1 | (byte2 << 7)`) is close to `1023` (`0x3ff`) — consistent with A0 tied to 3.3V and the ATmega's ADC maxing out. The final `printf '\xc0\x00'` disables reporting again so the channel doesn't keep streaming after the test.

If the bytes don't look like a valid `ANALOG_MESSAGE` at all (wrong leading byte, no bytes received, garbage), **stop and report BLOCKED** — don't guess at a decoding fix; this means either the wire protocol implementation or the device-node identification from Step 2 needs to be re-examined.

- [ ] **Step 5: Contrast test**

Ask the user to move the jumper from 3.3V to GND (or remove it entirely, leaving A0 floating), then repeat Step 4's send/read. Expected: a noticeably different (near-zero, or at least clearly different from 1023) value — confirms the reading tracks a real physical signal rather than being a coincidental fixed value.

- [ ] **Step 6: Update `CLAUDE.md`**

Add the confirmed bridge UART device node (from Step 2) to `CLAUDE.md`'s ATmega section, replacing the "Verify the exact node on-device" TODO language for this specific link with the actual confirmed node name.

- [ ] **Step 7: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: record confirmed ATmega bridge UART device node (Phase C verification)"
```

---

## Plan self-review notes

**Spec coverage:** Phase B's spec sections all map to a task here — message subset (`REPORT_ANALOG`/`ANALOG_MESSAGE`, Task 2's sketch), read semantics (the firmware side of subscribe/unsubscribe, Task 2's callback; the MT7688-side per-call wait/timeout logic is explicitly Phase D, not this plan), channel numbering 0-11 (Task 2's `ANALOG_CHANNEL_COUNT`), baud rate 57600 (Task 2's `Serial1.begin(57600)`), and all four "explicitly not designed here" items from the spec are each covered by an investigation step: Firmata library install/compile (Task 1 Steps 1-3), byte-encoding re-verification (Task 1 Step 4), bridge UART device node (Task 3 Step 2), and Phase D's tool signature (correctly left out of this plan entirely — it's Phase D's job, not Phase C's).

**Deliberate scope boundary:** this plan stops at "firmware sends/receives correct bytes over the raw bridge UART, verified by directly reading them from the MT7688 side with `stty`/`od`." It does **not** build the Go decoder or an MCP tool — that's Phase D, a separate future plan, matching how Phase A stopped at "toolchain proves out" without touching any protocol code.
