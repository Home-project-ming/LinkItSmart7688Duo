//go:build linux

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const i2cSlaveIoctl = 0x0703 // I2C_SLAVE, from <linux/i2c-dev.h>

// openI2C opens /dev/i2c-0 and binds it to addr via ioctl(I2C_SLAVE), the
// same small-helper-used-by-public-functions shape as exportGPIO in
// gpio.go. addr is assumed already validated by the caller (ReadI2C /
// WriteI2C in i2c.go validate the 7-bit range before this is ever
// reached).
func openI2C(addr byte) (*os.File, error) {
	f, err := os.OpenFile(i2cDevPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", i2cDevPath, err)
	}
	if err := unix.IoctlSetInt(int(f.Fd()), i2cSlaveIoctl, int(addr)); err != nil {
		f.Close()
		return nil, fmt.Errorf("set i2c slave address %#x: %w", addr, err)
	}
	return f, nil
}

func readI2C(addr byte, reg byte, length int) ([]byte, error) {
	f, err := openI2C(addr)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Write([]byte{reg}); err != nil {
		return nil, fmt.Errorf("write register %#x: %w", reg, err)
	}
	buf := make([]byte, length)
	n, err := f.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read %d bytes: %w", length, err)
	}
	return buf[:n], nil
}

func writeI2C(addr byte, reg byte, data []byte) error {
	f, err := openI2C(addr)
	if err != nil {
		return err
	}
	defer f.Close()

	payload := append([]byte{reg}, data...)
	if _, err := f.Write(payload); err != nil {
		return fmt.Errorf("write %d bytes to register %#x: %w", len(data), reg, err)
	}
	return nil
}
