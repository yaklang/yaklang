package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMCPServerEx(t *testing.T) {
	s, _ := NewMCPServer()

	if err := s.ServeSSE(":18083", "http://localhost:18083"); err != nil {
		panic(err)
	}
}

func TestMCPClient(t *testing.T) {
	log.SetLevel(log.FatalLevel)

	port := utils.GetRandomAvailableTCPPort()
	go func() {
		s, _ := NewMCPServer()

		if err := s.ServeSSE(fmt.Sprintf(":%d", port), fmt.Sprintf("http://localhost:%d", port)); err != nil {
			panic(err)
		}
	}()

	utils.WaitConnect(fmt.Sprintf("127.0.0.1:%d", port), 2)
	c, err := client.NewSSEMCPClient(fmt.Sprintf("http://localhost:%d/sse", port))
	require.NoError(t, err)

	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	err = c.Start(ctx)
	require.NoError(t, err)

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}
	c.Initialize(context.Background(), initRequest)

	request := mcp.CallToolRequest{}
	data := `{
"program": "Bash -i",
"shellType": "bash",
"ip": "127.0.0.1",
"port": 10000
}`
	request.Params.Name = "generate_reverse_shell_command"
	err = json.Unmarshal([]byte(data), &request.Params.Arguments)
	require.NoError(t, err)
	c.OnNotification(func(notification mcp.JSONRPCNotification) {
		fmt.Println("Call notification:", notification)
	})

	result, err := c.CallTool(context.Background(), request)
	require.NoError(t, err)
	for _, r := range result.Content {
		switch result := r.(type) {
		case mcp.TextContent:
			fmt.Println("CallResult:", result.Text)
		case mcp.ImageContent:
			fmt.Println("CallResult: image:\n" + result.Data)
		default:
			m, ok := result.(map[string]any)
			if ok {
				typ := utils.MapGetString(m, "type")
				if typ == "text" {
					fmt.Println("CallResult:", utils.MapGetString(m, "text"))
				} else if typ == "image" {
					fmt.Println("CallResult: image:\n" + utils.MapGetString(m, "data"))
				} else {
					spew.Dump(result)
				}
			} else {
				spew.Dump(result)
			}
		}
	}
}
