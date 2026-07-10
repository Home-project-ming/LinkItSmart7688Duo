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

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "linkit7688-hw", Version: "v0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{Name: "gpio_read", Description: "Read a GPIO pin's current value"}, gpioReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "gpio_write", Description: "Set a GPIO pin's output value. The pin should be set to \"out\" via gpio_set_direction first."}, gpioWriteHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "gpio_set_direction", Description: "Set a GPIO pin's direction (in/out)"}, gpioSetDirectionHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "i2c_read", Description: "Read bytes from an I2C device register on /dev/i2c-0; no repeated start, some devices may return stale data across the register-select/read boundary"}, i2cReadHandler)
	mcp.AddTool(server, &mcp.Tool{Name: "i2c_write", Description: "Write bytes to an I2C device register on /dev/i2c-0"}, i2cWriteHandler)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
