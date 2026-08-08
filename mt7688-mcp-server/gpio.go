// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// gpioSysfsPath is a var (not a const) so tests can point it at a temp
// directory holding a fake sysfs tree, exercising the export/direction/
// value plumbing without real hardware - same test-seam rationale as
// spiDevGlob in spi.go. Nothing in production ever reassigns it.
var gpioSysfsPath = "/sys/class/gpio"

// exportGPIO exports pin via the sysfs GPIO ABI. It is idempotent even
// under concurrent invocation: each MCP client session is a separate OS
// process, so two sessions can race to export the same not-yet-exported
// pin. A check-then-act (os.Stat then write) would be a TOCTOU race
// against that. Instead, this always attempts the export write and lets
// the kernel serialize it: if another process already exported the pin
// first, the write to .../export returns EBUSY, which we treat as
// success rather than an error.
func exportGPIO(pin int) error {
	f, err := os.OpenFile(gpioSysfsPath+"/export", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(strconv.Itoa(pin)); err != nil {
		if errors.Is(err, syscall.EBUSY) {
			return nil // already exported by a concurrent session
		}
		return fmt.Errorf("write export: %w", err)
	}
	return nil
}

func SetGPIODirection(pin int, direction string) error {
	if err := CheckBlocked(pin); err != nil {
		return err
	}
	if direction != "in" && direction != "out" {
		return fmt.Errorf("direction must be \"in\" or \"out\", got %q", direction)
	}
	if err := exportGPIO(pin); err != nil {
		return err
	}
	path := fmt.Sprintf("%s/gpio%d/direction", gpioSysfsPath, pin)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open direction: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(direction); err != nil {
		return fmt.Errorf("write direction: %w", err)
	}
	return nil
}

func ReadGPIO(pin int) (bool, error) {
	if err := CheckBlocked(pin); err != nil {
		return false, err
	}
	if err := exportGPIO(pin); err != nil {
		return false, err
	}
	path := fmt.Sprintf("%s/gpio%d/value", gpioSysfsPath, pin)
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read value: %w", err)
	}
	return strings.TrimSpace(string(data)) == "1", nil
}

func WriteGPIO(pin int, value bool) error {
	if err := CheckBlocked(pin); err != nil {
		return err
	}
	if err := exportGPIO(pin); err != nil {
		return err
	}
	path := fmt.Sprintf("%s/gpio%d/value", gpioSysfsPath, pin)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open value: %w", err)
	}
	defer f.Close()
	v := "0"
	if value {
		v = "1"
	}
	if _, err := f.WriteString(v); err != nil {
		return fmt.Errorf("write value: %w", err)
	}
	return nil
}
