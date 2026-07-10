// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

var blockedPins = map[int]string{
	12: "ATmega bridge UART (uartlite, ttyS0)",
	13: "ATmega bridge UART (uartlite, ttyS0)",
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
}

func CheckBlocked(pin int) error {
	if reason, blocked := blockedPins[pin]; blocked {
		return fmt.Errorf("gpio %d is blocked: %s", pin, reason)
	}
	return nil
}
