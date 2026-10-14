package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

// 回归测试：exec_codec 对畸形 workFlow 参数的健壮性
// （issue #5292：workFlow 传字符串数组导致 MCP worker panic / exit status 2）

// recordingCodecClient 记录 NewCodec 调用并返回可控结果
type recordingCodecClient struct {
	ypb.YakClient // 嵌入接口（仅 NewCodec 被实际调用）
	newCodecCalls []*ypb.CodecRequestFlow
	newCodecResp  *ypb.CodecResponse
	newCodecErr   error
}

func (c *recordingCodecClient) GetProfileDatabase() *gorm.DB { return nil }

func newRecordingCodecClient() *recordingCodecClient {
	return &recordingCodecClient{}
}

func (c *recordingCodecClient) NewCodec(ctx context.Context, in *ypb.CodecRequestFlow, _ ...grpc.CallOption) (*ypb.CodecResponse, error) {
	c.newCodecCalls = append(c.newCodecCalls, in)
	if c.newCodecErr != nil {
		return nil, c.newCodecErr
	}
	if c.newCodecResp != nil {
		return c.newCodecResp, nil
	}
	return &ypb.CodecResponse{Result: "encoded"}, nil
}

// callExecCodec 通过 handler 直接调用，返回 (result, error)
func callExecCodec(t *testing.T, client YakClientInterface, args string) (*mcp.CallToolResult, error) {
	t.Helper()
	request := mcp.CallToolRequest{}
	request.Params.Name = "exec_codec"
	if err := json.Unmarshal([]byte(args), &request.Params.Arguments); err != nil {
		t.Fatal(err)
	}
	handler := handleExecCodec(&MCPServer{grpcClient: client})
	return handler(context.Background(), request)
}

func TestExecCodecMalformedWorkFlowNoPanic(t *testing.T) {
	client := newRecordingCodecClient()

	t.Run("workFlow as plain string", func(t *testing.T) {
		result, err := callExecCodec(t, client, `{"text":"aGVsbG8=","workFlow":"base64decode"}`)
		require.Error(t, err)
		require.Nil(t, result)
		require.NotContains(t, err.Error(), "reflect")
	})

	t.Run("workFlow as string array (was panic / worker exit 2)", func(t *testing.T) {
		// 旧代码：mapstructure 部分解码后 req.WorkFlow 残留 nil 元素 → panic。
		// 此处若 panic，测试进程直接失败 —— 即回归证据。
		result, err := callExecCodec(t, client, `{"text":"aGVsbG8=","workFlow":["Base64Decode"]}`)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("workFlow with null item", func(t *testing.T) {
		result, err := callExecCodec(t, client, `{"text":"aGVsbG8=","workFlow":[null]}`)
		require.Error(t, err)
		require.Nil(t, result)
	})

	// 畸形输入不应触达 gRPC
	require.Empty(t, client.newCodecCalls, "畸形参数不应调用 NewCodec")
}

func TestExecCodecDecodeErrorShowsMethodDetails(t *testing.T) {
	client := newRecordingCodecClient()

	// 字符串数组含已知方法名 → 错误应包含该方法名的实际详情（而非仅通用列表）
	_, err := callExecCodec(t, client, `{"text":"aGVsbG8=","workFlow":["Base64Decode"]}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Base64Decode", "应包含请求中提到的方法名")
	require.Contains(t, err.Error(), "base64", "应包含 Base64Decode 的方法文档（非 not found）")
	require.NotContains(t, err.Error(), "method[Base64Decode] not found",
		"有效方法名不应报 not found")

	// 字符串数组含未知方法名 → 应包含 _available_methods 通用列表
	_, err = callExecCodec(t, client, `{"text":"aGVsbG8=","workFlow":["totally_bogus"]}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "totally_bogus")
	require.Contains(t, err.Error(), "_available_methods", "未知方法应附可用方法名列表")
	require.Contains(t, err.Error(), "Base64Encode", "可用方法列表应包含已知方法")

	// 结构数组但 codecType 未知 → 同样应包含该方法名和可用列表
	_, err = callExecCodec(t, client, `{"text":"aGVsbG8=","workFlow":[{"codecType":"wrongCase"}]}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrongCase")
	require.Contains(t, err.Error(), "_available_methods")
}

func TestExecCodecUnknownCodecTypeReadableError(t *testing.T) {
	client := newRecordingCodecClient()
	// 旧代码：未知 codecType 直透 codec 引擎，报 reflect 零值错误（不可读）。
	result, err := callExecCodec(t, client,
		`{"text":"aGVsbG8=","workFlow":[{"codecType":"base64Decode"}]}`)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "not found")
	require.NotContains(t, err.Error(), "reflect.Value.Type",
		"未知方法应返回可读的 not found 错误而非 reflect 错误")
	// 不应触达 gRPC
	require.Empty(t, client.newCodecCalls, "未知 codecType 应在 MCP 层被拦截")
}

func TestExecCodecValidRequestUnchanged(t *testing.T) {
	// 合法请求（结构数组 + 枚举名）→ 应成功调用 NewCodec 并返回结果
	client := newRecordingCodecClient()
	client.newCodecResp = &ypb.CodecResponse{Result: "68656c6c6f", RawResult: []byte("Njg2NTZjNmM2Zg==")}

	result, err := callExecCodec(t, client,
		`{"text":"hello","workFlow":[{"codecType":"HexEncode"}]}`)
	require.NoError(t, err, "合法请求不应被前置校验拒绝")
	require.NotNil(t, result)

	// 验证 NewCodec 收到正确的工作流
	require.Len(t, client.newCodecCalls, 1, "应恰好调用一次 NewCodec")
	call := client.newCodecCalls[0]
	require.Equal(t, "hello", call.Text)
	require.Len(t, call.WorkFlow, 1)
	require.Equal(t, "HexEncode", call.WorkFlow[0].CodecType)
}
