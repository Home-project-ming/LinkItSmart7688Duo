// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

// atmegaBridgeDevPath is the ATmega UART bridge, confirmed live during
// Phase C via dmesg's pinmux allocation log (uart0 group -> pins 12/13 ->
// 10000c00.uartlite -> ttyS0) - a different physical UART from the one
// uart_transact uses (ttyS1, pins 45/46).
const atmegaBridgeDevPath = "/dev/ttyS0"

// atmegaBridgeBaud is fixed, not caller-adjustable like uart_transact's
// baud_rate - the ATmega firmware's Firmata.begin(Serial1) is hardcoded
// to this rate (see atmega-firmware/adc-bridge/adc-bridge.ino), so a
// caller-supplied mismatch would just desync communication with no
// benefit.
const atmegaBridgeBaud = 57600

// atmegaChannelCount matches ANALOG_CHANNEL_COUNT in the ATmega firmware
// (confirmed against the real board's analog_pin_to_channel_PGM array
// during Phase B/C - 12 channels, A0-A11).
const atmegaChannelCount = 12

func validateAtmegaADCRead(channel int, timeoutMs int) error {
	if channel < 0 || channel >= atmegaChannelCount {
		return fmt.Errorf("channel must be 0-%d, got %d", atmegaChannelCount-1, channel)
	}
	if timeoutMs < 1 || timeoutMs > 10000 {
		return fmt.Errorf("timeout_ms must be 1-10000, got %d", timeoutMs)
	}
	return nil
}

// AtmegaADCRead validates input then performs the enable/wait/disable
// sequence against atmegaBridgeDevPath. Real termios/read-loop work is in
// atmega_linux.go (atmega_other.go stubs it out on non-linux dev
// machines), mirroring the i2c.go/spi.go/uart.go platform splits.
func AtmegaADCRead(channel int, timeoutMs int) (int, error) {
	if err := validateAtmegaADCRead(channel, timeoutMs); err != nil {
		return 0, err
	}
	return atmegaADCRead(byte(channel), timeoutMs)
}

func validateAtmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) error {
	if len(txData) > 256 {
		return fmt.Errorf("tx_data length must be 0-256, got %d", len(txData))
	}
	if timeoutMs < 0 || timeoutMs > 25500 {
		return fmt.Errorf("timeout_ms must be 0-25500, got %d", timeoutMs)
	}
	if maxReadBytes < 1 || maxReadBytes > 4096 {
		return fmt.Errorf("max_read_bytes must be 1-4096, got %d", maxReadBytes)
	}
	return nil
}

// AtmegaBridgeRaw validates input then performs a write-then-timed-read
// transaction against atmegaBridgeDevPath, structurally identical to
// UARTTransact but fixed to this bridge's device and baud rate.
func AtmegaBridgeRaw(txData []byte, timeoutMs int, maxReadBytes int) ([]byte, error) {
	if err := validateAtmegaBridgeRaw(txData, timeoutMs, maxReadBytes); err != nil {
		return nil, err
	}
	return atmegaBridgeRaw(txData, timeoutMs, maxReadBytes)
}
