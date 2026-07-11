// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

// Phase A toolchain bring-up test: blinks the ATmega's onboard TX LED
// via the TXLED0/TXLED1 macros (this board's pins_arduino.h doesn't
// define LED_BUILTIN - confirmed absent from the vendor's own variant
// file during implementation - but does define these standard Arduino
// Leonardo-family macros for the 32U4's USB-activity TX indicator LED,
// used as direct statements rather than digitalWrite/pinMode calls).
// Does not touch Serial1 (the bridge UART) or communicate with the
// MT7688 side at all - this sketch exists purely to prove the
// compile-and-flash path works, per docs/superpowers/specs/
// 2026-07-11-atmega-phase-a-toolchain.md.

void setup() {
}

void loop() {
  TXLED1;
  delay(500);
  TXLED0;
  delay(500);
}
