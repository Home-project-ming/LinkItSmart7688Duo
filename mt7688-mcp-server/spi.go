// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"fmt"
	"path/filepath"
)

// validateSPITransfer runs before any device access, matching the
// ReadI2C/WriteI2C pattern in i2c.go.
func validateSPITransfer(mode int, speedHz int, bitsPerWord int, txData []byte) error {
	if mode < 0 || mode > 3 {
		return fmt.Errorf("mode must be 0-3, got %d", mode)
	}
	if bitsPerWord < 1 || bitsPerWord > 32 {
		return fmt.Errorf("bits_per_word must be 1-32, got %d", bitsPerWord)
	}
	if speedHz <= 0 {
		return fmt.Errorf("speed_hz must be positive, got %d", speedHz)
	}
	if len(txData) == 0 || len(txData) > 256 {
		return fmt.Errorf("tx_data length must be 1-256, got %d", len(txData))
	}
	return nil
}

// spiDevGlob is a var (not a const) so tests can point it at a temp
// directory instead of /dev, exercising the 0/1/many-matches logic
// without needing real hardware.
var spiDevGlob = "/dev/spidev*"

// discoverSPIDevice finds the sole SPI device node on the system. Only
// one exists on this board today (/dev/spidev32766.1 - see design doc),
// with a kernel-internal bus number an LLM caller has no way to guess.
// Returns an error naming all matches if there isn't exactly one, so a
// second device (if one is ever added) fails loudly instead of picking
// one arbitrarily.
func discoverSPIDevice() (string, error) {
	matches, err := filepath.Glob(spiDevGlob)
	if err != nil {
		return "", fmt.Errorf("glob %s: %w", spiDevGlob, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no SPI device node found matching %s", spiDevGlob)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple SPI device nodes found %v: spi_transfer needs exactly one, this tool doesn't yet support selecting between them", matches)
	}
	return matches[0], nil
}

// --- MIPS ioctl request-number derivation ---
//
// MIPS uses a different _IOC bit layout than the generic one most
// internet-sourced ioctl constants assume (arch/mips/include/uapi/asm/
// ioctl.h vs include/uapi/asm-generic/ioctl.h in the Linux kernel
// source - shared with powerpc/sparc, the historical "BSD-style" ioctl
// encoding). Generic (x86/arm): SIZEBITS=14, dir values NONE=0/WRITE=1/
// READ=2. MIPS: SIZEBITS=13, dir values NONE=1/READ=2/WRITE=4. Getting
// this wrong doesn't fail loudly - it silently targets the wrong ioctl
// number. These constants are derived from the formula directly (not
// copied from an x86/generic reference) and cross-checked against
// hand-derived values in spi_test.go.
const (
	mipsIOCNRBITS   = 8
	mipsIOCTYPEBITS = 8
	mipsIOCSIZEBITS = 13

	mipsIOCNRSHIFT   = 0
	mipsIOCTYPESHIFT = mipsIOCNRSHIFT + mipsIOCNRBITS
	mipsIOCSIZESHIFT = mipsIOCTYPESHIFT + mipsIOCTYPEBITS
	mipsIOCDIRSHIFT  = mipsIOCSIZESHIFT + mipsIOCSIZEBITS

	mipsIOCNone  = 1
	mipsIOCRead  = 2
	mipsIOCWrite = 4
)

func mipsIOC(dir, typ, nr, size uintptr) uintptr {
	return (dir << mipsIOCDIRSHIFT) | (typ << mipsIOCTYPESHIFT) | (nr << mipsIOCNRSHIFT) | (size << mipsIOCSIZESHIFT)
}

const spiIOCMagic = 'k' // SPI_IOC_MAGIC, from <linux/spi/spidev.h>

// spiIOCTransferSize is sizeof(struct spi_ioc_transfer) from
// <linux/spi/spidev.h>: two 8-byte pointers (tx_buf, rx_buf), two 4-byte
// fields (len, speed_hz), one 2-byte field (delay_usecs), and six 1-byte
// fields (bits_per_word, cs_change, tx_nbits, rx_nbits, word_delay_usecs,
// pad) = 8+8+4+4+2+1+1+1+1+1+1 = 32 bytes. Stable uapi, unchanged since
// spidev was added to the kernel.
const spiIOCTransferSize = 32

var (
	spiIOCWRMode   = mipsIOC(mipsIOCWrite, spiIOCMagic, 1, 1)                  // SPI_IOC_WR_MODE
	spiIOCMessage1 = mipsIOC(mipsIOCWrite, spiIOCMagic, 0, spiIOCTransferSize) // SPI_IOC_MESSAGE(1)
)

// TransferSPI validates input, discovers the sole SPI device node, and
// performs a full-duplex transfer. Real ioctl work is in spi_linux.go
// (spi_other.go stubs it out on non-linux dev machines), mirroring the
// i2c.go / i2c_linux.go / i2c_other.go split.
func TransferSPI(mode int, speedHz int, bitsPerWord int, txData []byte) (rxData []byte, device string, err error) {
	if err := validateSPITransfer(mode, speedHz, bitsPerWord, txData); err != nil {
		return nil, "", err
	}
	device, err = discoverSPIDevice()
	if err != nil {
		return nil, "", err
	}
	rxData, err = transferSPI(device, mode, speedHz, bitsPerWord, txData)
	if err != nil {
		return nil, "", err
	}
	return rxData, device, nil
}
