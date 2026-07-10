package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type PingInput struct {
	Message string `json:"message" jsonschema:"text to echo back"`
}

type PingOutput struct {
	Reply string `json:"reply" jsonschema:"the echoed text"`
}

func pingHandler(ctx context.Context, req *mcp.CallToolRequest, input PingInput) (*mcp.CallToolResult, PingOutput, error) {
	return nil, PingOutput{Reply: input.Message}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "linkit7688-hw", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "echo a message back"}, pingHandler)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
