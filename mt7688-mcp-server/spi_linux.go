// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

//go:build linux && (mips || mipsle)

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// transferSPI opens device, sets the SPI mode via SPI_IOC_WR_MODE, then
// issues a single full-duplex transfer via SPI_IOC_MESSAGE(1). The
// spi_ioc_transfer struct is built by hand into a 32-byte buffer
// (spiIOCTransferSize, see spi.go) rather than a Go struct, since the
// kernel ABI's tx_buf/rx_buf fields are raw 64-bit pointers with no safe
// Go-native representation. txData/rxData must stay alive and unmoved
// until the ioctl returns - Go's current garbage collector doesn't move
// heap objects, and both slices remain referenced by local variables for
// the whole function, so this holds.
func transferSPI(device string, mode int, speedHz int, bitsPerWord int, txData []byte) ([]byte, error) {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	defer f.Close()

	if err := unix.IoctlSetPointerInt(int(f.Fd()), uint(spiIOCWRMode), mode); err != nil {
		return nil, fmt.Errorf("set spi mode %d on %s: %w", mode, device, err)
	}

	rxData := make([]byte, len(txData))
	xfer := make([]byte, spiIOCTransferSize)
	binary.LittleEndian.PutUint64(xfer[0:8], uint64(uintptr(unsafe.Pointer(&txData[0]))))  // tx_buf
	binary.LittleEndian.PutUint64(xfer[8:16], uint64(uintptr(unsafe.Pointer(&rxData[0])))) // rx_buf
	binary.LittleEndian.PutUint32(xfer[16:20], uint32(len(txData)))                        // len
	binary.LittleEndian.PutUint32(xfer[20:24], uint32(speedHz))                            // speed_hz
	// xfer[24:26] delay_usecs = 0
	xfer[26] = byte(bitsPerWord)
	// xfer[27:32] cs_change, tx_nbits, rx_nbits, word_delay_usecs, pad = 0

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), spiIOCMessage1, uintptr(unsafe.Pointer(&xfer[0])))
	runtime.KeepAlive(txData)
	runtime.KeepAlive(rxData)
	if errno != 0 {
		return nil, fmt.Errorf("spi transfer on %s (mode=%d, speed_hz=%d, bits_per_word=%d, len=%d): %w", device, mode, speedHz, bitsPerWord, len(txData), errno)
	}
	return rxData, nil
}
