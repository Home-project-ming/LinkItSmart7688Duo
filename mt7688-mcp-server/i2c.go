package main

import "fmt"

const i2cDevPath = "/dev/i2c-0"

// ReadI2C reads length bytes starting at register reg from the 7-bit I2C
// address addr on /dev/i2c-0. Validation runs before any device access so
// invalid input never touches the OS-specific ioctl path (see readI2C,
// which is only implemented for linux - see i2c_linux.go).
func ReadI2C(addr byte, reg byte, length int) ([]byte, error) {
	if addr > 0x7f {
		return nil, fmt.Errorf("i2c address %#x out of 7-bit range", addr)
	}
	if length <= 0 || length > 256 {
		return nil, fmt.Errorf("length must be 1-256, got %d", length)
	}
	return readI2C(addr, reg, length)
}

// WriteI2C writes data to register reg on the 7-bit I2C address addr on
// /dev/i2c-0. Validation runs before any device access, matching ReadI2C.
func WriteI2C(addr byte, reg byte, data []byte) error {
	if addr > 0x7f {
		return fmt.Errorf("i2c address %#x out of 7-bit range", addr)
	}
	if len(data) == 0 || len(data) > 256 {
		return fmt.Errorf("data length must be 1-256, got %d", len(data))
	}
	return writeI2C(addr, reg, data)
}
