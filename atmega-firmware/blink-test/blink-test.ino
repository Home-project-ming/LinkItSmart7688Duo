// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

// Phase A toolchain bring-up test: blinks the board's physical "LED
// (D13)" - confirmed from the vendor's own MediaTek LinkIt Smart 7688
// Developer's Guide (Figure 7 board photo + Figure 10 pin-out diagram)
// to be a real, populated LED wired to Arduino digital pin D13 on the
// ATmega32U4 side, near the MCU reset button. An earlier version of
// this sketch used the TXLED0/TXLED1 macros (the 32U4's USB-activity
// indicator), which compiled and flashed successfully but produced no
// visible blink - most likely because this board doesn't populate that
// particular LED, unlike the vendor-confirmed D13 LED. Plain pin 13 is
// used directly (not a LED_BUILTIN alias, which this board's
// pins_arduino.h doesn't define) since the vendor documentation
// confirms D13 by Arduino pin number, not by macro name.
// Does not touch Serial1 (the bridge UART) or communicate with the
// MT7688 side at all - this sketch exists purely to prove the
// compile-and-flash path works, per docs/superpowers/specs/
// 2026-07-11-atmega-phase-a-toolchain.md.

void setup() {
  pinMode(13, OUTPUT);
}

void loop() {
  digitalWrite(13, HIGH);
  delay(500);
  digitalWrite(13, LOW);
  delay(500);
}
