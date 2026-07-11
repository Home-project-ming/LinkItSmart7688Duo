// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

var blockedPins = map[int]string{
	6:  "SPI CS1 (spi_cs1 pinmux group)",
	7:  "SPI bus (fixed-function pin, no GPIO alternative)",
	8:  "SPI bus (fixed-function pin, no GPIO alternative)",
	9:  "SPI bus (fixed-function pin, no GPIO alternative)",
	10: "SPI bus (fixed-function pin, no GPIO alternative)",
	12: "ATmega bridge UART (uartlite, ttyS0)",
	13: "ATmega bridge UART (uartlite, ttyS0)",
	18: "PWM channel 0 (pwm0 pinmux group)",
	19: "PWM channel 1 (pwm1 pinmux group)",
	20: "console",
	21: "console",
	22: "SDXC/extroot",
	23: "SDXC/extroot",
	24: "SDXC/extroot",
	25: "SDXC/extroot",
	26: "SDXC/extroot",
	27: "SDXC/extroot",
	28: "SDXC/extroot",
	29: "SDXC/extroot",
	38: "ATmega ISP reset (held by the core)",
	45: "UART1 (uart1 pinmux group)",
	46: "UART1 (uart1 pinmux group)",
}

func CheckBlocked(pin int) error {
	if reason, blocked := blockedPins[pin]; blocked {
		return fmt.Errorf("gpio %d is blocked: %s", pin, reason)
	}
	return nil
}
