// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

// XCS20 slave-serial download engine, driven from the MT7688 over the
// Firmata bridge.
//
// Why this firmware exists: the download was originally designed to run
// from MT7688 GPIO (see the FPGA_XCS20 repo's slave-serial design doc),
// but the Duo breaks out only P1, P8 and P9, and P8/P9 are the console
// UART -- one free pin against the four outputs and one input the
// adapter needs. The ATmega has the pins, so it drives the adapter and
// the 7688 only ships bytes over the existing 57600-baud bridge.
//
// Why the whole sequence lives here rather than on the 7688: driving one
// pin per bridge message would be 534,432 round trips. The host sends
// BEGIN, a few hundred DATA chunks and END; everything time-critical --
// the /PROGRAM pulse, the clocking, the startup sequence -- happens on
// this side, which is also the only side that can actually read DONE.
//
// This sketch does NOT implement the ADC reporting that adc-bridge.ino
// does. Only one of the two can be flashed at a time; see the
// implementation spec for the tradeoff.
//
// Runs entirely over Serial1 (the MT7688 bridge UART, per CLAUDE.md) at
// 57600 baud, the Firmata library's own default. Never touches Serial
// (the USB CDC debug console) - mixing the two would corrupt bridge
// traffic, the same rule every other ATmega-side sketch here follows.

#include <Firmata.h>

// Adapter wiring. Four outputs into the 74LS125 level shifter, one input
// back through the 10K:15K divider. These are frozen in the hardware, so
// unlike the old MT7688 path they are constants rather than tool inputs.
//
//   D8  -> 1A -> 1Y -> DB25.2  (PD0 / DIN)
//   D9  -> 2A -> 2Y -> DB25.3  (PD1 / CCLK)
//   D10 -> 3A -> 3Y -> DB25.4  (PD2 / /PROGRAM)
//   D11 -> 4A -> 4Y -> DB25.5  (PD3 / buffer gate; LOW hands us the bus)
//   D12 <- 10K:15K divider <- DB25.13 (SLCT / DONE)
#define PIN_DIN   8
#define PIN_CCLK  9
#define PIN_PROG  10
#define PIN_GATE  11
#define PIN_DONE  12

// SysEx command and subcommands, mirrored in fpga_bridge.go.
#define SYSEX_FPGA   0x7E
#define SUB_BEGIN    0x01
#define SUB_DATA     0x02
#define SUB_END      0x03
#define SUB_ABORT    0x04

// Status codes, mirrored in fpga_bridge.go's fpgaStatus* constants.
#define STATUS_OK             0x00
#define STATUS_DONE_STUCK     0x01
#define STATUS_BAD_LENGTH     0x02
#define STATUS_NOT_STREAMING  0x03

#define CHUNK_MAX 49

// Timing. Slave-serial CCLK has no minimum frequency (the configuration
// logic is fully static) and a maximum near 8MHz, so these are chosen
// with margins wide enough that datasheet precision does not matter.
#define PROGRAM_LOW_HOLD_MS   10   // spec minimum is sub-microsecond
#define PROGRAM_CLEAR_WAIT_MS 100  // INIT is not wired out, so wait blind
#define STARTUP_CLOCKS        16

// Cached port registers. digitalWrite costs a few microseconds per call,
// and a full chunk is 1176 of them -- enough to eat into the ~10ms the
// bridge takes to deliver the next chunk. Resolving the register and
// mask once at setup keeps the inner loop to a single store, without
// hardcoding a pin-to-port mapping this board's variant might not share
// with a stock Leonardo.
static volatile uint8_t *dinPort, *cclkPort, *progPort, *gatePort;
static volatile uint8_t *donePin;
static uint8_t dinMask, cclkMask, progMask, gateMask, doneMask;

static inline void pinHigh(volatile uint8_t *port, uint8_t mask) { *port |= mask; }
static inline void pinLow(volatile uint8_t *port, uint8_t mask) { *port &= ~mask; }
static inline void pinSet(volatile uint8_t *port, uint8_t mask, bool high) {
  if (high) *port |= mask; else *port &= ~mask;
}
static inline bool doneIsHigh() { return (*donePin & doneMask) != 0; }

static bool streaming = false;
static uint32_t receivedBytes = 0;

// sendStatus replies with the layout parseReply expects:
//   F0 7E <sub> <status> <done> <n0> <n1> <n2> <n3> F7
// The count travels as four 7-bit groups because SysEx data bytes cannot
// carry bit 7.
static void sendStatus(byte sub, byte status, bool done, uint32_t n) {
  byte body[7];
  body[0] = sub;
  body[1] = status;
  body[2] = done ? 1 : 0;
  for (byte i = 0; i < 4; i++) {
    body[3 + i] = (byte)((n >> (7 * i)) & 0x7F);
  }
  Firmata.sendSysex(SYSEX_FPGA, sizeof(body), body);
}

// parkBus puts every signal in its safe idle state: buffers Hi-Z, so
// nothing on this side is driving the FPGA's config pins.
static void parkBus() {
  pinHigh(gatePort, gateMask);  // PD3 high = 74LS125 outputs Hi-Z
  pinHigh(progPort, progMask);  // /PROGRAM idle high
  pinLow(cclkPort, cclkMask);
  pinHigh(dinPort, dinMask);
}

static void doBegin() {
  parkBus();
  pinLow(gatePort, gateMask);  // take the bus

  pinLow(progPort, progMask);
  delay(PROGRAM_LOW_HOLD_MS);
  pinHigh(progPort, progMask);

  // DONE must have dropped. If it did not, either the gate is not
  // reaching the buffers or DONE readback is not connected, and
  // streaming 22KB into that would tell us nothing.
  if (doneIsHigh()) {
    parkBus();
    streaming = false;
    sendStatus(SUB_BEGIN, STATUS_DONE_STUCK, true, 0);
    return;
  }

  delay(PROGRAM_CLEAR_WAIT_MS);
  streaming = true;
  receivedBytes = 0;
  sendStatus(SUB_BEGIN, STATUS_OK, false, 0);
}

// doData decodes one chunk and clocks it straight out. It deliberately
// sends no reply: at 57600 baud a chunk takes ~10ms to arrive and well
// under 1ms to shift out, so the host can stream continuously and a
// per-chunk acknowledgement would only add latency. Losses show up in
// the byte count END reports.
static void doData(byte *argv, byte argc) {
  if (!streaming || argc < 1) return;

  byte n = argv[0];
  if (n < 1 || n > CHUNK_MAX) return;
  byte encodedLen = (byte)((n * 8 + 6) / 7);
  if (argc < 1 + encodedLen) return;

  byte *enc = argv + 1;
  byte payload[CHUNK_MAX];
  memset(payload, 0, n);
  // Mirror of encode7bit in fpga_bridge.go: a dense MSB-first bit
  // stream, 7 bits per wire byte, rather than Firmata's usual
  // one-byte-per-nibble scheme that would double the transfer.
  for (uint16_t bit = 0; bit < (uint16_t)n * 8; bit++) {
    byte v = (enc[bit / 7] >> (6 - (bit % 7))) & 1;
    payload[bit / 8] |= v << (7 - (bit % 8));
  }

  for (byte i = 0; i < n; i++) {
    byte b = payload[i];
    for (int8_t shift = 7; shift >= 0; shift--) {
      pinSet(dinPort, dinMask, (b >> shift) & 1);
      pinHigh(cclkPort, cclkMask);  // DIN is sampled on the rising edge
      pinLow(cclkPort, cclkMask);
    }
  }
  receivedBytes += n;
}

static void doEnd() {
  if (!streaming) {
    sendStatus(SUB_END, STATUS_NOT_STREAMING, false, receivedBytes);
    return;
  }

  // Startup: DIN held high, extra clocks, watching DONE come up. All of
  // them are issued even once DONE rises -- the startup sequence has
  // further internal steps (I/O enable, GSR release) on the same clock.
  pinHigh(dinPort, dinMask);
  bool done = false;
  for (byte i = 0; i < STARTUP_CLOCKS; i++) {
    pinHigh(cclkPort, cclkMask);
    pinLow(cclkPort, cclkMask);
    if (doneIsHigh()) done = true;
  }

  parkBus();
  streaming = false;
  sendStatus(SUB_END, STATUS_OK, done, receivedBytes);
}

// doAbort is the host's unconditional bus release, sent on every exit
// path including the successful one. It sends no reply: the host writes
// it from a deferred cleanup and closes the port without reading.
static void doAbort() {
  parkBus();
  streaming = false;
}

void sysexCallback(byte command, byte argc, byte *argv) {
  if (command != SYSEX_FPGA || argc < 1) return;

  switch (argv[0]) {
    case SUB_BEGIN: doBegin(); break;
    case SUB_DATA:  doData(argv + 1, argc - 1); break;
    case SUB_END:   doEnd(); break;
    case SUB_ABORT: doAbort(); break;
  }
}

void setup() {
  pinMode(PIN_DIN, OUTPUT);
  pinMode(PIN_CCLK, OUTPUT);
  pinMode(PIN_PROG, OUTPUT);
  pinMode(PIN_GATE, OUTPUT);
  pinMode(PIN_DONE, INPUT);

  dinPort = portOutputRegister(digitalPinToPort(PIN_DIN));
  dinMask = digitalPinToBitMask(PIN_DIN);
  cclkPort = portOutputRegister(digitalPinToPort(PIN_CCLK));
  cclkMask = digitalPinToBitMask(PIN_CCLK);
  progPort = portOutputRegister(digitalPinToPort(PIN_PROG));
  progMask = digitalPinToBitMask(PIN_PROG);
  gatePort = portOutputRegister(digitalPinToPort(PIN_GATE));
  gateMask = digitalPinToBitMask(PIN_GATE);
  donePin = portInputRegister(digitalPinToPort(PIN_DONE));
  doneMask = digitalPinToBitMask(PIN_DONE);

  // Park before the bridge is even up, so a reset mid-download cannot
  // leave the buffers driving the FPGA's config pins.
  parkBus();

  Serial1.begin(57600);
  Firmata.begin(Serial1);
  Firmata.attach(START_SYSEX, sysexCallback);
}

void loop() {
  while (Firmata.available()) {
    Firmata.processInput();
  }
}
