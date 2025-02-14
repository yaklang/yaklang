package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/mcp/transport/stdio"
)

type HelloArguments struct {
	Submitter string `json:"submitter" jsonschema:"required,description=The name of the thing calling this tool (openai or google or claude etc)'"`
}

// This is explained in the docs at https://mcpgolang.com/tools
func main() {
	done := make(chan struct{})
	server := mcp.NewServer(stdio.NewStdioServerTransport())
	err := server.RegisterTool("hello", "Say hello to a person", func(arguments HelloArguments) (*mcp.ToolResponse, error) {
		return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Hello, %s!", arguments.Submitter))), nil
	})
	err = server.Serve()
	if err != nil {
		panic(err)
	}
	<-done
}
