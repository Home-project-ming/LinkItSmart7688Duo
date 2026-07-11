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
// an earlier phase). 12 is confirmed directly from this board's real
// analog_pin_to_channel_PGM array (12 entries, A0-A11), and separately
// confirmed as Firmata's own hardcoded TOTAL_ANALOG_PINS for
// __AVR_ATmega32U4__ in Firmata's Boards.h (independent of
// NUM_ANALOG_INPUTS entirely for this chip).
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
