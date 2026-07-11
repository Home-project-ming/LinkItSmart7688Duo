// Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GPIOReadInput struct {
	Pin int `json:"pin" jsonschema:"GPIO pin number to read"`
}
type GPIOReadOutput struct {
	Value bool `json:"value" jsonschema:"true if the pin reads high"`
}

func gpioReadHandler(ctx context.Context, req *mcp.CallToolRequest, input GPIOReadInput) (*mcp.CallToolResult, GPIOReadOutput, error) {
	v, err := ReadGPIO(input.Pin)
	if err != nil {
		return nil, GPIOReadOutput{}, err
	}
	return nil, GPIOReadOutput{Value: v}, nil
}

type GPIOWriteInput struct {
	Pin   int  `json:"pin" jsonschema:"GPIO pin number to write"`
	Value bool `json:"value" jsonschema:"true to drive the pin high, false for low"`
}
type GPIOWriteOutput struct {
	OK bool `json:"ok"`
}

func gpioWriteHandler(ctx context.Context, req *mcp.CallToolRequest, input GPIOWriteInput) (*mcp.CallToolResult, GPIOWriteOutput, error) {
	if err := WriteGPIO(input.Pin, input.Value); err != nil {
		return nil, GPIOWriteOutput{}, err
	}
	return nil, GPIOWriteOutput{OK: true}, nil
}

type GPIODirectionInput struct {
	Pin       int    `json:"pin" jsonschema:"GPIO pin number"`
	Direction string `json:"direction" jsonschema:"\"in\" or \"out\""`
}
type GPIODirectionOutput struct {
	OK bool `json:"ok"`
}

func gpioSetDirectionHandler(ctx context.Context, req *mcp.CallToolRequest, input GPIODirectionInput) (*mcp.CallToolResult, GPIODirectionOutput, error) {
	if err := SetGPIODirection(input.Pin, input.Direction); err != nil {
		return nil, GPIODirectionOutput{}, err
	}
	return nil, GPIODirectionOutput{OK: true}, nil
}

type I2CReadInput struct {
	Addr   byte `json:"addr" jsonschema:"7-bit I2C device address (0-127)"`
	Reg    byte `json:"reg" jsonschema:"register address to read from"`
	Length int  `json:"length" jsonschema:"number of bytes to read (1-256)"`
}
type I2CReadOutput struct {
	Data []int `json:"data" jsonschema:"bytes read from the device"`
}

func i2cReadHandler(ctx context.Context, req *mcp.CallToolRequest, input I2CReadInput) (*mcp.CallToolResult, I2CReadOutput, error) {
	data, err := ReadI2C(input.Addr, input.Reg, input.Length)
	if err != nil {
		return nil, I2CReadOutput{}, err
	}
	out := make([]int, len(data))
	for i, b := range data {
		out[i] = int(b)
	}
	return nil, I2CReadOutput{Data: out}, nil
}

type I2CWriteInput struct {
	Addr byte  `json:"addr" jsonschema:"7-bit I2C device address (0-127)"`
	Reg  byte  `json:"reg" jsonschema:"register address to write to"`
	Data []int `json:"data" jsonschema:"bytes to write (each 0-255)"`
}
type I2CWriteOutput struct {
	OK bool `json:"ok"`
}

func i2cWriteHandler(ctx context.Context, req *mcp.CallToolRequest, input I2CWriteInput) (*mcp.CallToolResult, I2CWriteOutput, error) {
	data := make([]byte, len(input.Data))
	for i, v := range input.Data {
		if v < 0 || v > 255 {
			return nil, I2CWriteOutput{}, fmt.Errorf("data[%d] out of byte range (0-255): %d", i, v)
		}
		data[i] = byte(v)
	}
	if err := WriteI2C(input.Addr, input.Reg, data); err != nil {
		return nil, I2CWriteOutput{}, err
	}
	return nil, I2CWriteOutput{OK: true}, nil
}

type SPITransferInput struct {
	Mode        int   `json:"mode" jsonschema:"SPI mode 0-3 (CPOL/CPHA)"`
	SpeedHz     int   `json:"speed_hz" jsonschema:"clock speed in Hz"`
	BitsPerWord int   `json:"bits_per_word" jsonschema:"bits per word, 1-32"`
	TxData      []int `json:"tx_data" jsonschema:"bytes to send (each 0-255); rx_data comes back the same length"`
}
type SPITransferOutput struct {
	RxData []int  `json:"rx_data" jsonschema:"bytes received during the transfer, same length as tx_data"`
	Device string `json:"device" jsonschema:"the /dev/spidev* node this transfer used"`
}

func spiTransferHandler(ctx context.Context, req *mcp.CallToolRequest, input SPITransferInput) (*mcp.CallToolResult, SPITransferOutput, error) {
	txData := make([]byte, len(input.TxData))
	for i, v := range input.TxData {
		if v < 0 || v > 255 {
			return nil, SPITransferOutput{}, fmt.Errorf("tx_data[%d] out of byte range (0-255): %d", i, v)
		}
		txData[i] = byte(v)
	}
	rxData, device, err := TransferSPI(input.Mode, input.SpeedHz, input.BitsPerWord, txData)
	if err != nil {
		return nil, SPITransferOutput{}, err
	}
	out := make([]int, len(rxData))
	for i, b := range rxData {
		out[i] = int(b)
	}
	return nil, SPITransferOutput{RxData: out, Device: device}, nil
}

type PWMConfigureInput struct {
	Channel  int `json:"channel" jsonschema:"PWM channel: 0 (GPIO18) or 1 (GPIO19)"`
	PeriodNs int `json:"period_ns" jsonschema:"PWM period in nanoseconds"`
	DutyNs   int `json:"duty_ns" jsonschema:"PWM duty cycle in nanoseconds, must be 0-period_ns"`
}
type PWMConfigureOutput struct {
	OK bool `json:"ok"`
}

func pwmConfigureHandler(ctx context.Context, req *mcp.CallToolRequest, input PWMConfigureInput) (*mcp.CallToolResult, PWMConfigureOutput, error) {
	if err := ConfigurePWM(input.Channel, input.PeriodNs, input.DutyNs); err != nil {
		return nil, PWMConfigureOutput{}, err
	}
	return nil, PWMConfigureOutput{OK: true}, nil
}

type PWMSetEnabledInput struct {
	Channel int  `json:"channel" jsonschema:"PWM channel: 0 (GPIO18) or 1 (GPIO19)"`
	Enabled bool `json:"enabled" jsonschema:"true to start the PWM output, false to stop it"`
}
type PWMSetEnabledOutput struct {
	OK bool `json:"ok"`
}

func pwmSetEnabledHandler(ctx context.Context, req *mcp.CallToolRequest, input PWMSetEnabledInput) (*mcp.CallToolResult, PWMSetEnabledOutput, error) {
	if err := SetPWMEnabled(input.Channel, input.Enabled); err != nil {
		return nil, PWMSetEnabledOutput{}, err
	}
	return nil, PWMSetEnabledOutput{OK: true}, nil
}

type PWMReadInput struct {
	Channel int `json:"channel" jsonschema:"PWM channel: 0 (GPIO18) or 1 (GPIO19)"`
}
type PWMReadOutput struct {
	PeriodNs int    `json:"period_ns" jsonschema:"current period in nanoseconds"`
	DutyNs   int    `json:"duty_ns" jsonschema:"current duty cycle in nanoseconds"`
	Enabled  bool   `json:"enabled" jsonschema:"true if the PWM output is currently running"`
	Polarity string `json:"polarity" jsonschema:"\"normal\" or \"inversed\""`
}

func pwmReadHandler(ctx context.Context, req *mcp.CallToolRequest, input PWMReadInput) (*mcp.CallToolResult, PWMReadOutput, error) {
	state, err := ReadPWM(input.Channel)
	if err != nil {
		return nil, PWMReadOutput{}, err
	}
	return nil, PWMReadOutput{
		PeriodNs: state.PeriodNs,
		DutyNs:   state.DutyNs,
		Enabled:  state.Enabled,
		Polarity: state.Polarity,
	}, nil
}

type UARTTransactInput struct {
	BaudRate      int   `json:"baud_rate" jsonschema:"baud rate, 1-2500000"`
	TxData        []int `json:"tx_data" jsonschema:"bytes to send (each 0-255); may be empty for a read-only/listen call"`
	ReadTimeoutMs int   `json:"read_timeout_ms" jsonschema:"how long to wait for a reply, in milliseconds (0-25500, rounded down to the nearest 100ms); 0 means non-blocking"`
	MaxReadBytes  int   `json:"max_read_bytes" jsonschema:"maximum bytes to read, 1-4096"`
}
type UARTTransactOutput struct {
	RxData []int `json:"rx_data" jsonschema:"bytes received within the timeout window; may be shorter than max_read_bytes or empty if nothing arrived"`
}

func uartTransactHandler(ctx context.Context, req *mcp.CallToolRequest, input UARTTransactInput) (*mcp.CallToolResult, UARTTransactOutput, error) {
	txData := make([]byte, len(input.TxData))
	for i, v := range input.TxData {
		if v < 0 || v > 255 {
			return nil, UARTTransactOutput{}, fmt.Errorf("tx_data[%d] out of byte range (0-255): %d", i, v)
		}
		txData[i] = byte(v)
	}
	rxData, err := UARTTransact(input.BaudRate, txData, input.ReadTimeoutMs, input.MaxReadBytes)
	if err != nil {
		return nil, UARTTransactOutput{}, err
	}
	out := make([]int, len(rxData))
	for i, b := range rxData {
		out[i] = int(b)
	}
	return nil, UARTTransactOutput{RxData: out}, nil
}

type I2SPlayInput struct {
	WavBase64 string `json:"wav_base64" jsonschema:"base64-encoded WAV audio to play (max 100000 decoded bytes, roughly 6 seconds at 8kHz mono)"`
}
type I2SPlayOutput struct {
	OK bool `json:"ok"`
}

func i2sPlayHandler(ctx context.Context, req *mcp.CallToolRequest, input I2SPlayInput) (*mcp.CallToolResult, I2SPlayOutput, error) {
	if err := PlayI2S(input.WavBase64); err != nil {
		return nil, I2SPlayOutput{}, err
	}
	return nil, I2SPlayOutput{OK: true}, nil
}

type AtmegaADCReadInput struct {
	Channel   int `json:"channel" jsonschema:"analog channel to read, 0-11 (A0-A11)"`
	TimeoutMs int `json:"timeout_ms" jsonschema:"how long to wait for a reading, in milliseconds (1-10000)"`
}
type AtmegaADCReadOutput struct {
	Value int `json:"value" jsonschema:"the ATmega's raw 10-bit ADC reading, 0-1023"`
}

func atmegaADCReadHandler(ctx context.Context, req *mcp.CallToolRequest, input AtmegaADCReadInput) (*mcp.CallToolResult, AtmegaADCReadOutput, error) {
	value, err := AtmegaADCRead(input.Channel, input.TimeoutMs)
	if err != nil {
		return nil, AtmegaADCReadOutput{}, err
	}
	return nil, AtmegaADCReadOutput{Value: value}, nil
}

type AtmegaBridgeRawInput struct {
	TxData       []int `json:"tx_data" jsonschema:"raw bytes to send over the ATmega bridge (each 0-255); may be empty for a read-only/listen call"`
	TimeoutMs    int   `json:"timeout_ms" jsonschema:"how long to wait for a reply, in milliseconds (0-25500, rounded down to the nearest 100ms); 0 means non-blocking"`
	MaxReadBytes int   `json:"max_read_bytes" jsonschema:"maximum bytes to read, 1-4096"`
}
type AtmegaBridgeRawOutput struct {
	RxData []int `json:"rx_data" jsonschema:"bytes received within the timeout window; may be shorter than max_read_bytes or empty if nothing arrived"`
}

func atmegaBridgeRawHandler(ctx context.Context, req *mcp.CallToolRequest, input AtmegaBridgeRawInput) (*mcp.CallToolResult, AtmegaBridgeRawOutput, error) {
	txData := make([]byte, len(input.TxData))
	for i, v := range input.TxData {
		if v < 0 || v > 255 {
			return nil, AtmegaBridgeRawOutput{}, fmt.Errorf("tx_data[%d] out of byte range (0-255): %d", i, v)
		}
		txData[i] = byte(v)
	}
	rxData, err := AtmegaBridgeRaw(txData, input.TimeoutMs, input.MaxReadBytes)
	if err != nil {
		return nil, AtmegaBridgeRawOutput{}, err
	}
	out := make([]int, len(rxData))
	for i, b := range rxData {
		out[i] = int(b)
	}
	return nil, AtmegaBridgeRawOutput{RxData: out}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "linkit7688-hw", Version: "v0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{Name: "gpio_read", Description: "Read a GPIO pin's current value"}, gpioReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "gpio_write", Description: "Set a GPIO pin's output value. The pin should be set to \"out\" via gpio_set_direction first."}, gpioWriteHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "gpio_set_direction", Description: "Set a GPIO pin's direction (in/out)"}, gpioSetDirectionHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "i2c_read", Description: "Read bytes from an I2C device register on /dev/i2c-0; no repeated start, some devices may return stale data across the register-select/read boundary"}, i2cReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "i2c_write", Description: "Write bytes to an I2C device register on /dev/i2c-0"}, i2cWriteHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "spi_transfer", Description: "Full-duplex SPI transfer on the board's sole SPI device node. Send tx_data (dummy 0x00 bytes for a read-only transfer), get back rx_data of the same length."}, spiTransferHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "pwm_configure", Description: "Set a PWM channel's period and duty cycle in nanoseconds. Only channels 0 (GPIO18) and 1 (GPIO19) are supported."}, pwmConfigureHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "pwm_set_enabled", Description: "Start or stop a PWM channel's output. Call pwm_configure first to set a period/duty."}, pwmSetEnabledHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "pwm_read", Description: "Read a PWM channel's current period, duty cycle, enabled state, and polarity"}, pwmReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "uart_transact", Description: "Write bytes to /dev/ttyS1 (uart1, pins 45/46) and read back whatever arrives within read_timeout_ms. tx_data may be empty for a read-only/listen call. A timeout with no data is not an error - it means nothing was received in the window."}, uartTransactHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "i2s_play", Description: "Play a WAV audio clip through the board's speaker/headphone output. Blocks until playback completes. Max 100000 bytes of decoded WAV data (roughly 6 seconds at 8kHz mono). Recording is not available - the board's capture path has a confirmed driver-level bug that hangs the device."}, i2sPlayHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "atmega_adc_read", Description: "Read one analog channel (0-11, A0-A11) from the ATmega32U4 side over the UART bridge (/dev/ttyS0), using the Firmata REPORT_ANALOG/ANALOG_MESSAGE protocol. Blocks until a reading for that channel arrives or timeout_ms elapses."}, atmegaADCReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "atmega_bridge_raw", Description: "Send raw bytes to the ATmega32U4 over the UART bridge (/dev/ttyS0, fixed 57600 baud) and read back whatever arrives within timeout_ms. Low-level escape hatch for speaking Firmata (or any future ATmega firmware protocol) directly; most callers should use atmega_adc_read instead."}, atmegaBridgeRawHandler)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
