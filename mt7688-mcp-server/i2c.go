// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import "fmt"

const i2cDevPath = "/dev/i2c-0"

// ReadI2C reads length bytes starting at register reg from the 7-bit I2C
// address addr on /dev/i2c-0. Validation runs before any device access so
// invalid input never touches the OS-specific ioctl path (see readI2C,
// which is only implemented for linux - see i2c_linux.go).
//
// Known limitations (inherited from the plan's design, not fixed in v1
// since this is meant to be a generic low-level primitive, not a
// device-specific driver):
//
//   - No repeated start. The register-select write and the data read are
//     two separate I2C messages (each STOP-terminated), issued as two
//     plain write(2)/read(2) syscalls rather than a single combined
//     transaction via ioctl(I2C_RDWR) with two i2c_msgs. Most simple
//     register-mapped devices tolerate the intervening STOP fine, but
//     some documented devices require a repeated start between the
//     register-select and the read, and will silently return
//     stale/wrong data (not an error) if there's a STOP in between.
//   - No locking across the write+read pair. Each individual syscall is
//     atomic, but the open/write/read/close sequence as a whole is not:
//     two concurrent MCP sessions targeting the same device address
//     could interleave their register-select write and data read.
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
