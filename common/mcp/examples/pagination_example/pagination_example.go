package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/mcp/transport/stdio"
)

// Arguments for our tools
type HelloArguments struct {
	Name string `json:"name" jsonschema:"required,description=The name to say hello to"`
}

type ByeArguments struct {
	Name string `json:"name" jsonschema:"required,description=The name to say goodbye to"`
}

// Arguments for our prompts
type GreetingArguments struct {
	Language string `json:"language" jsonschema:"required,description=The language to greet in"`
}

type FarewellArguments struct {
	Language string `json:"language" jsonschema:"required,description=The language to say farewell in"`
}

func main() {
	// Create a new server with pagination enabled (2 items per page)
	server := mcp.NewServer(
		stdio.NewStdioServerTransport(),
		mcp.WithPaginationLimit(2),
	)
	err := server.Serve()
	if err != nil {
		panic(err)
	}

	// Register multiple tools
	toolNames := []string{"hello1", "hello2", "hello3", "bye1", "bye2"}
	for _, name := range toolNames[:3] {
		err = server.RegisterTool(name, "Say hello to someone", func(args HelloArguments) (*mcp.ToolResponse, error) {
			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Hello, %s!", args.Name))), nil
		})
		if err != nil {
			panic(err)
		}
	}
	for _, name := range toolNames[3:] {
		err = server.RegisterTool(name, "Say goodbye to someone", func(args ByeArguments) (*mcp.ToolResponse, error) {
			return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Goodbye, %s!", args.Name))), nil
		})
		if err != nil {
			panic(err)
		}
	}

	// Register multiple prompts
	promptNames := []string{"greet1", "greet2", "greet3", "farewell1", "farewell2"}
	for _, name := range promptNames[:3] {
		err = server.RegisterPrompt(name, "Greeting in different languages", func(args GreetingArguments) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse("test", mcp.NewPromptMessage(mcp.NewTextContent(fmt.Sprintf("Hello in %s!", args.Language)), mcp.RoleUser)), nil
		})
		if err != nil {
			panic(err)
		}
	}
	for _, name := range promptNames[3:] {
		err = server.RegisterPrompt(name, "Farewell in different languages", func(args FarewellArguments) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse("test", mcp.NewPromptMessage(mcp.NewTextContent(fmt.Sprintf("Goodbye in %s!", args.Language)), mcp.RoleUser)), nil
		})
		if err != nil {
			panic(err)
		}
	}

	// Register multiple resources
	resourceNames := []string{"resource1.txt", "resource2.txt", "resource3.txt", "resource4.txt", "resource5.txt"}
	for i, name := range resourceNames {
		content := fmt.Sprintf("This is resource %d", i+1)
		err = server.RegisterResource(
			name,
			fmt.Sprintf("Resource %d", i+1),
			fmt.Sprintf("Description for resource %d", i+1),
			"text/plain",
			func() (*mcp.ResourceResponse, error) {
				return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource(name, content, "text/plain")), nil
			},
		)
		if err != nil {
			panic(err)
		}
	}

	// Keep the server running
	select {}
}
