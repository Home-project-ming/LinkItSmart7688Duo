// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux

package main

import "fmt"

// readI2C/writeI2C on non-linux build hosts (e.g. Windows, where this
// module is developed and cross-compiled from). golang.org/x/sys/unix
// only implements IoctlSetInt for darwin/dragonfly/freebsd/hurd/linux/
// netbsd/openbsd/aix/solaris - not windows - so the real ioctl(I2C_SLAVE)
// implementation in i2c_linux.go cannot compile here. These stubs exist
// solely so `go build`/`go test` succeed on a non-linux dev machine; they
// are never exercised by i2c_test.go, since ReadI2C/WriteI2C in i2c.go
// validate address/length and return before calling into these. The real
// target is always GOOS=linux GOARCH=mipsle (see build.ps1), where
// i2c_linux.go is compiled in instead of this file.
func readI2C(addr byte, reg byte, length int) ([]byte, error) {
	return nil, fmt.Errorf("i2c is only supported on linux (target device); this build is not linux")
}

func writeI2C(addr byte, reg byte, data []byte) error {
	return fmt.Errorf("i2c is only supported on linux (target device); this build is not linux")
}
