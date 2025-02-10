package main

import (
	"context"
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/mcp/transport/http"
)

func main() {
	// Create an HTTP transport that connects to the server
	transport := http.NewHTTPClientTransport("/mcp")
	transport.WithBaseURL("http://localhost:8080/api/v1")
	// Public metoro token - not a leak
	transport.WithHeader("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjdXN0b21lcklkIjoiOThlZDU1M2QtYzY4ZC00MDRhLWFhZjItNDM2ODllNWJiMGUzIiwiZW1haWwiOiJ0ZXN0QGNocmlzYmF0dGFyYmVlLmNvbSIsImV4cCI6MTgyMTI0NzIzN30.QeFzKsP1yO16pVol0mkAdt7qhJf6nTqBoqXqdWawBdE")

	// Create a new client with the transport
	client := mcp.NewClient(transport)

	// Initialize the client
	if resp, err := client.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	} else {
		log.Printf("Initialized client: %v", spew.Sdump(resp))
	}

	// List available tools
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	log.Println("Available Tools:")
	for _, tool := range tools.Tools {
		desc := ""
		if tool.Description != nil {
			desc = *tool.Description
		}
		log.Printf("Tool: %s. Description: %s", tool.Name, desc)
	}

	response, err := client.CallTool(context.Background(), "get_log_attributes", map[string]interface{}{})
	if err != nil {
		log.Fatalf("Failed to call get_log_attributes tool: %v", err)
	}

	log.Printf("Response: %v", spew.Sdump(response))
}
