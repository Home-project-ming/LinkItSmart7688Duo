// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build !linux || (!mips && !mipsle)

package main

import "fmt"

// transferSPI on build hosts that aren't linux/mips or linux/mipsle (e.g.
// Windows, where this module is developed and cross-compiled from, or
// linux/amd64). This stub exists solely so `go build`/`go test` succeed
// off-target; the real target is always GOOS=linux GOARCH=mipsle (see
// build.ps1), where spi_linux.go is compiled in instead of this file - its
// ioctl constants are derived from MIPS-specific _IOC bit layout and would
// be wrong on other Linux architectures, hence the narrower build tag on
// spi_linux.go and the matching complement here. This stub is never
// exercised by any test, since TransferSPI in spi.go validates input and
// discovers the device, returning before calling into this. Matches
// i2c_other.go's existing rationale.
func transferSPI(device string, mode int, speedHz int, bitsPerWord int, txData []byte) ([]byte, error) {
	return nil, fmt.Errorf("spi is only supported on linux (target device); this build is not linux")
}
