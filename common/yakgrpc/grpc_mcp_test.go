package yakgrpc

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 测试前设置
func init() {
	mcp.RegisterNewLocalClient(func(locals ...bool) (mcp.YakClientInterface, error) {
		client, err := NewLocalClient(locals...)
		if err != nil {
			return nil, err
		}
		v, ok := client.(mcp.YakClientInterface)
		if !ok {
			return nil, utils.Error("failed to cast client to yakgrpc.Client")
		}
		return v, nil
	})
}

func TestGRPC_StartMcpServer_BasicFlow(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建启动请求
	req := &ypb.StartMcpServerRequest{
		Host:      "127.0.0.1",
		Port:      0, // 使用随机端口
		EnableAll: true,
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	var responses []*ypb.StartMcpServerResponse
	var serverUrl string

	// 接收前几个状态消息
	for i := 0; i < 3; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.NotNil(t, resp)
		responses = append(responses, resp)

		if resp.ServerUrl != "" {
			serverUrl = resp.ServerUrl
		}

		log.Infof("Received MCP response: Status=%s, Message=%s, ServerUrl=%s",
			resp.Status, resp.Message, resp.ServerUrl)

		if resp.Status == "running" {
			break
		}
	}

	// 验证收到的响应
	require.GreaterOrEqual(t, len(responses), 2)

	// 第一个响应应该是 starting 状态
	require.Equal(t, "starting", responses[0].Status)
	require.Contains(t, responses[0].Message, "Initializing MCP server")

	// 应该有一个 configured 状态
	configuredFound := false
	runningFound := false
	for _, resp := range responses {
		if resp.Status == "configured" {
			configuredFound = true
		}
		if resp.Status == "running" {
			runningFound = true
			require.NotEmpty(t, resp.ServerUrl, "ServerUrl should be set when status is running")
		}
	}
	require.True(t, configuredFound, "Should receive configured status")
	require.True(t, runningFound, "Should receive running status")
	require.NotEmpty(t, serverUrl, "ServerUrl should be provided")

	// 验证 URL 格式
	require.Contains(t, serverUrl, "http://127.0.0.1:", "ServerUrl should contain correct host")
}

func TestGRPC_StartMcpServer_DefaultPort(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 不指定端口，应该使用随机端口
	req := &ypb.StartMcpServerRequest{
		// 不设置 Host 和 Port，使用默认值
		Tool: []string{"codec"},
	}

	stream, err := client.StartMcpServer(ctx, req)
	require.NoError(t, err)

	// 接收前几个状态消息
	for i := 0; i < 3; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if resp.Status == "running" {
			require.NotEmpty(t, resp.ServerUrl)
			require.Contains(t, resp.ServerUrl, "http://127.0.0.1:", "Should use default host")
			break
		}
	}
}
