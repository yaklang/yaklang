package yakgrpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestStartSSEMCP(t *testing.T) {
	grpcClient, err := NewLocalClient()
	require.NoError(t, err)

	stream, err := grpcClient.StartSSEMCP(context.Background(), &ypb.StartMCPRequest{
		Tools: []string{"codec"},
	})
	require.NoError(t, err)

	defer stream.CloseSend()

	msg, err := stream.Recv()
	require.NoError(t, err)

	mcpClient, err := client.NewSSEMCPClient(msg.GetURL())
	require.NoError(t, err)

	defer mcpClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err = mcpClient.Start(ctx)
	require.NoError(t, err)

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}
	mcpClient.Initialize(context.Background(), initRequest)

	request := mcp.CallToolRequest{}
	data := `{
  "method": ["Base64Encode"]
}`
	request.Params.Name = "codec_method_details"
	err = json.Unmarshal([]byte(data), &request.Params.Arguments)
	require.NoError(t, err)

	result, err := mcpClient.CallTool(ctx, request)
	require.NoError(t, err)
	spew.Dump(result)
}
