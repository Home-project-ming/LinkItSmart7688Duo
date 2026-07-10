package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const gpioSysfsPath = "/sys/class/gpio"

func exportGPIO(pin int) error {
	path := fmt.Sprintf("%s/gpio%d", gpioSysfsPath, pin)
	if _, err := os.Stat(path); err == nil {
		return nil // already exported
	}
	f, err := os.OpenFile(gpioSysfsPath+"/export", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(strconv.Itoa(pin)); err != nil {
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
