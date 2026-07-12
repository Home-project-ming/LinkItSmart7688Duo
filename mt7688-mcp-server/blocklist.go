// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

var blockedPins = map[int]string{
	0: "I2S (i2s pinmux group, WM8960 codec)",
	1: "I2S (i2s pinmux group, WM8960 codec)",
	2: "I2S (i2s pinmux group, WM8960 codec)",
	// Pin 3 has two independent reasons to stay blocked: it is part of the
	// i2s pinmux group, and it is ALSO the ATmega32U4's ISP reset line
	// (the active linuxgpio entry in the board's /etc/avrdude.conf reads
	// "reset = 3"; corroborated by the vendor Developer's Guide pin-out).
	// Driving it resets the ATmega mid-operation, which would break the
	// atmega_adc_read/atmega_bridge_raw tools' firmware underneath them.
	3:  "I2S (i2s pinmux group) + ATmega ISP reset line (driving it resets the ATmega)",
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
	// Previously mislabeled "ATmega ISP reset" (that is pin 3, see above).
	// Live-verified 2026-07-12: /sys/kernel/debug/gpio shows gpio-38 held
	// by the kernel as input "reset" — the device tree's
	// gpio-keys-polled/wps node (label "reset", bank1 offset 6 = 38,
	// linux,code KEY_WPS_BUTTON), i.e. the board's Wi-Fi Reset button.
	38: "Wi-Fi reset button input (held by the kernel, gpio-keys \"reset\")",
	45: "UART1 (uart1 pinmux group)",
	46: "UART1 (uart1 pinmux group)",
}

func CheckBlocked(pin int) error {
	if reason, blocked := blockedPins[pin]; blocked {
		return fmt.Errorf("gpio %d is blocked: %s", pin, reason)
	}
	return nil
}
