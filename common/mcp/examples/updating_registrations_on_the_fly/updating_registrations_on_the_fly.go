package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/mcp/transport/stdio"
	"time"
)

type HelloArguments struct {
	Submitter string `json:"submitter" jsonschema:"required,description=The name of the thing calling this tool (openai or google or claude etc)'"`
}

type Content struct {
	Title       string  `json:"title" jsonschema:"required,description=The title to submit"`
	Description *string `json:"description" jsonschema:"description=The description to submit"`
}

// This is a stupid server that demonstrates how to update registrations on the fly.
// Every second the server will register a new tool, a new prompt and a new resource, then unregister the old ones.
func main() {
	done := make(chan struct{})
	server := mcp.NewServer(stdio.NewStdioServerTransport())
	err := server.Serve()
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			err := server.RegisterTool("hello", "Say hello to a person", func(arguments HelloArguments) (*mcp.ToolResponse, error) {
				return mcp.NewToolResponse(mcp.NewTextContent(fmt.Sprintf("Hello, %s!", arguments.Submitter))), nil
			})
			if err != nil {
				panic(err)
			}
			time.Sleep(1 * time.Second)
			err = server.DeregisterTool("hello")
			if err != nil {
				panic(err)
			}
		}
	}()
	go func() {
		for {

			err = server.RegisterPrompt("prompt_test", "This is a test prompt", func(arguments Content) (*mcp.PromptResponse, error) {
				return mcp.NewPromptResponse("description", mcp.NewPromptMessage(mcp.NewTextContent(fmt.Sprintf("Hello, %server!", arguments.Title)), mcp.RoleUser)), nil
			})
			if err != nil {
				panic(err)
			}
			time.Sleep(1 * time.Second)
			err = server.DeregisterPrompt("prompt_test")
			if err != nil {
				panic(err)
			}
		}

	}()
	go func() {
		err = server.RegisterResource("test://resource", "resource_test", "This is a test resource", "application/json", func() (*mcp.ResourceResponse, error) {
			return mcp.NewResourceResponse(mcp.NewTextEmbeddedResource("test://resource", "This is a test resource", "application/json")), nil
		})
		if err != nil {
			panic(err)
		}
		time.Sleep(1 * time.Second)
		err = server.DeregisterResource("test://resource")
		if err != nil {
			panic(err)
		}
	}()

	<-done
}
