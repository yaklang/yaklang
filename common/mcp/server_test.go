package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMCPServerEx(t *testing.T) {
	s := NewMCPServer()

	if err := s.ServeSSE(":18083", "http://localhost:18083"); err != nil {
		panic(err)
	}
}

func TestMCPClient(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	go func() {
		s := NewMCPServer()

		if err := s.ServeSSE(fmt.Sprintf(":%d", port), fmt.Sprintf("http://localhost:%d", port)); err != nil {
			panic(err)
		}
	}()

	utils.WaitConnect(fmt.Sprintf("127.0.0.1:%d", port), 2)
	c, err := client.NewSSEMCPClient(fmt.Sprintf("http://localhost:%d/sse", port))
	require.NoError(t, err)

	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
  "code": "a = cli.Int(\"a\", cli.setRequired(true))\nb = cli.Int(\"b\", cli.setRequired(true))\ncli.check()\nyakit.Output(string(a+b))",
  "execParams": [
    {"key": "a", "value": "1"},
    {"key": "b", "value": "2"}
  ],
  "pluginType": "yak"
}`
	request.Params.Name = "exec_yak_script"
	err = json.Unmarshal([]byte(data), &request.Params.Arguments)
	require.NoError(t, err)

	result, err := c.CallTool(context.Background(), request)
	require.NoError(t, err)
	for _, r := range result.Content {
		switch result := r.(type) {
		case mcp.TextContent:
			fmt.Println(result.Text)
		case mcp.ImageContent:
			fmt.Println("image:\n" + result.Data)
		default:
			spew.Dump(result)
		}
	}
}
