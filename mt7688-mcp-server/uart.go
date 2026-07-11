// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

// uartDevPath is the sole free, pin-routed general-purpose UART on this
// board (uart1, pins 45/46, confirmed via dmesg: "10000d00.uart1: ttyS1
// ... base_baud = 2500000" - see design doc). uart0 (ttyS0, pins 12/13) is
// the ATmega bridge and uart2 (ttyS2, pins 20/21) is the console; both are
// already GPIO-blocklisted (Task 1) and neither is touched by this file.
// Hardcoded, not discovered like SPI's device path - unlike SPI's
// dynamically-numbered bus, this is a stable kernel-assigned path and
// there's exactly one candidate.
const uartDevPath = "/dev/ttyS1"

// uartBaseBaud is uart1's real hardware base_baud (16550A UART, confirmed
// via dmesg - see design doc), used to bound the baud_rate parameter to
// something the hardware can actually represent rather than leaving it
// unbounded.
const uartBaseBaud = 2500000

// validateUARTTransact runs before any device access, matching the
// pattern in i2c.go/spi.go. Unlike spi.go's validateSPITransfer, tx_data
// length 0 is valid here (a read-only/listen call) - UART's write and
// read aren't structurally symmetric the way SPI's are.
func validateUARTTransact(baudRate int, txData []byte, readTimeoutMs int, maxReadBytes int) error {
	if baudRate < 1 || baudRate > uartBaseBaud {
		return fmt.Errorf("baud_rate must be 1-%d, got %d", uartBaseBaud, baudRate)
	}
	if len(txData) > 256 {
		return fmt.Errorf("tx_data length must be 0-256, got %d", len(txData))
	}
	if readTimeoutMs < 0 || readTimeoutMs > 25500 {
		return fmt.Errorf("read_timeout_ms must be 0-25500, got %d", readTimeoutMs)
	}
	if maxReadBytes < 1 || maxReadBytes > 4096 {
		return fmt.Errorf("max_read_bytes must be 1-4096, got %d", maxReadBytes)
	}
	return nil
}
