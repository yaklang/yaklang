package mcp

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	mcpserver "github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

type scriptResultClient struct {
	YakClientInterface
	stream ypb.Yak_DebugPluginClient
}

func (c *scriptResultClient) DebugPlugin(context.Context, *ypb.DebugPluginRequest, ...grpc.CallOption) (ypb.Yak_DebugPluginClient, error) {
	return c.stream, nil
}

type scriptResultStream struct {
	grpc.ClientStream
	messages []*ypb.ExecResult
	err      error
}

func (s *scriptResultStream) Recv() (*ypb.ExecResult, error) {
	if len(s.messages) == 0 {
		return nil, s.err
	}
	message := s.messages[0]
	s.messages = s.messages[1:]
	return message, nil
}

func TestExecYakScriptStreamError(t *testing.T) {
	for _, tt := range []struct {
		name      string
		output    string
		terminal  error
		wantError bool
	}{
		{name: "empty success", terminal: io.EOF},
		{name: "successful output", output: "script output", terminal: io.EOF},
		{name: "panic", terminal: errors.New("YakVM Panic: test panic"), wantError: true},
		{name: "output before failure", output: "partial output", terminal: errors.New("execution failed"), wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stream := &scriptResultStream{err: tt.terminal}
			if tt.output != "" {
				stream.messages = []*ypb.ExecResult{{IsMessage: true, Message: []byte(tt.output)}}
			}
			srv := &MCPServer{
				server:     mcpserver.NewMCPServer("script-result-test", "1"),
				grpcClient: &scriptResultClient{stream: stream},
			}
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"pluginType": "yak", "code": "test"}
			result, err := handleExecYakScript(srv)(context.Background(), request)
			require.NoError(t, err, "execution failures belong in the tool result")
			require.NotNil(t, result)
			require.Equal(t, tt.wantError, result.IsError)
			var contents []string
			for _, item := range result.Content {
				text, ok := mcp.AsTextContent(item)
				require.True(t, ok)
				contents = append(contents, text.Text)
			}
			if tt.output != "" {
				require.Contains(t, contents, tt.output, "preserve output produced before failure")
			}
			if tt.wantError {
				require.Contains(t, contents, "[Error] "+tt.terminal.Error())
			} else if tt.output == "" {
				require.Equal(t, []string{"[System] Script execution completed with no output"}, contents)
			}
		})
	}
}
