package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"context"

	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/mcp/transport/stdio"
)

func main() {
	// Start the server process
	log.Info("Starting server... in ./server/main.go stdio")
	execName, _ := filepath.Abs("testserver")
	log.Info("exec.Command(testserver): " + execName)
	// cmd := exec.Command(execName)
	cmd := exec.Command("go", "run", "./server/main.go")
	stdoutReader, stdoutWriter := utils.NewBufPipe(nil)
	cmd.Stdout = io.MultiWriter(stdoutWriter, os.Stdout)

	stdinReader, stdinWriter := utils.NewBufPipe(nil)
	cmd.Stdin = stdinReader

	go func() {
		log.Info("cmd.Start() to start server")
		if raw, err := cmd.CombinedOutput(); err != nil {
			fmt.Println(string(raw))
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	time.Sleep(3 * time.Second)
	log.Info("start to create stdio server trans with io")
	clientTransport := stdio.NewStdioServerTransportWithIO(stdoutReader, stdinWriter)

	log.Info("start to create mcp.NewClient with stdio trans mcp server")
	client := mcp.NewClient(clientTransport)

	log.Info("start to call Initialize in client")
	if _, err := client.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	// List available tools
	log.Info("start to listTolls with context.Background()")
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}
	spew.Dump(tools)

	log.Println("Available Tools:")
	for _, tool := range tools.Tools {
		desc := ""
		if tool.Description != nil {
			desc = *tool.Description
		}
		log.Printf("Tool: %s. Description: %s", tool.Name, desc)
	}

	// Example of calling the hello tool
	helloArgs := map[string]interface{}{
		"name": "World",
	}

	log.Println("\nCalling hello tool:")
	helloResponse, err := client.CallTool(context.Background(), "hello", helloArgs)
	if err != nil {
		log.Printf("Failed to call hello tool: %v", err)
	} else if helloResponse != nil && len(helloResponse.Content) > 0 && helloResponse.Content[0].TextContent != nil {
		log.Printf("Hello response: %s", helloResponse.Content[0].TextContent.Text)
	}

	// Example of calling the calculate tool
	calcArgs := map[string]interface{}{
		"operation": "add",
		"a":         10,
		"b":         5,
	}

	log.Println("\nCalling calculate tool:")
	calcResponse, err := client.CallTool(context.Background(), "calculate", calcArgs)
	if err != nil {
		log.Printf("Failed to call calculate tool: %v", err)
	} else if calcResponse != nil && len(calcResponse.Content) > 0 && calcResponse.Content[0].TextContent != nil {
		log.Printf("Calculate response: %s", calcResponse.Content[0].TextContent.Text)
	}

	// Example of calling the time tool
	timeArgs := map[string]interface{}{
		"format": "2006-01-02 15:04:05",
	}

	log.Println("\nCalling time tool:")
	timeResponse, err := client.CallTool(context.Background(), "time", timeArgs)
	if err != nil {
		log.Printf("Failed to call time tool: %v", err)
	} else if timeResponse != nil && len(timeResponse.Content) > 0 && timeResponse.Content[0].TextContent != nil {
		log.Printf("Time response: %s", timeResponse.Content[0].TextContent.Text)
	}

	// List available prompts
	prompts, err := client.ListPrompts(context.Background(), nil)
	if err != nil {
		log.Printf("Failed to list prompts: %v", err)
	} else {
		log.Println("\nAvailable Prompts:")
		for _, prompt := range prompts.Prompts {
			desc := ""
			if prompt.Description != nil {
				desc = *prompt.Description
			}
			log.Printf("Prompt: %s. Description: %s", prompt.Name, desc)
		}

		// Example of using the uppercase prompt
		promptArgs := map[string]interface{}{
			"input": "Hello, Model Context Protocol!",
		}

		log.Printf("\nCalling uppercase prompt:")
		upperResponse, err := client.GetPrompt(context.Background(), "uppercase", promptArgs)
		if err != nil {
			log.Printf("Failed to get uppercase prompt: %v", err)
		} else if upperResponse != nil && len(upperResponse.Messages) > 0 && upperResponse.Messages[0].Content != nil {
			log.Printf("Uppercase response: %s", upperResponse.Messages[0].Content.TextContent.Text)
		}

		// Example of using the reverse prompt
		log.Printf("\nCalling reverse prompt:")
		reverseResponse, err := client.GetPrompt(context.Background(), "reverse", promptArgs)
		if err != nil {
			log.Printf("Failed to get reverse prompt: %v", err)
		} else if reverseResponse != nil && len(reverseResponse.Messages) > 0 && reverseResponse.Messages[0].Content != nil {
			log.Printf("Reverse response: %s", reverseResponse.Messages[0].Content.TextContent.Text)
		}
	}
}
