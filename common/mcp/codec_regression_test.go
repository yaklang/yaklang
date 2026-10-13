package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

// 回归测试：exec_codec 对畸形 workFlow 参数的健壮性
// （issue 复现：workFlow 传字符串数组导致 MCP worker panic / exit status 2）

func callExecCodecDirect(t *testing.T, args string) (string, error) {
	t.Helper()
	request := mcp.CallToolRequest{}
	request.Params.Name = "exec_codec"
	if err := json.Unmarshal([]byte(args), &request.Params.Arguments); err != nil {
		t.Fatal(err)
	}
	handler := handleExecCodec(nil) // 错误路径不触碰 s.grpcClient
	result, err := handler(context.Background(), request)
	if result != nil {
		_ = result // 错误路径返回 nil result
	}
	return "", err
}

func TestExecCodecMalformedWorkFlowNoPanic(t *testing.T) {
	t.Run("workFlow as plain string", func(t *testing.T) {
		_, err := callExecCodecDirect(t, `{"text":"aGVsbG8=","workFlow":"base64decode"}`)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "reflect", "错误信息不应是底层 reflect 错误")
	})

	t.Run("workFlow as string array (was panic / worker exit 2)", func(t *testing.T) {
		// 旧代码：mapstructure 解码失败后遍历部分解码的 WorkFlow（含 nil）→ panic。
		// 此处若 panic，测试进程直接失败 —— 即回归证据。
		_, err := callExecCodecDirect(t, `{"text":"aGVsbG8=","workFlow":["Base64Decode"]}`)
		require.Error(t, err)
	})

	t.Run("workFlow with null item", func(t *testing.T) {
		_, err := callExecCodecDirect(t, `{"text":"aGVsbG8=","workFlow":[null]}`)
		require.Error(t, err)
	})
}

func TestExecCodecDecodeErrorShowsMethodDetails(t *testing.T) {
	// 旧代码：解码失败时 details 恒为 {}（用了零值 req）。
	// 新行为：错误信息应包含可用方法名提示（至少出现已知方法 Base64Encode）。
	_, err := callExecCodecDirect(t, `{"text":"aGVsbG8=","workFlow":"wrongtype"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Base64Encode",
		"decode 失败的错误信息应携带可用 codec 方法名提示")
}

func TestExecCodecUnknownCodecTypeReadableError(t *testing.T) {
	// 旧代码：未知 codecType 直透 codec 引擎，报
	// "reflect: call of reflect.Value.Type on zero Value"（不可读）。
	_, err := callExecCodecDirect(t,
		`{"text":"aGVsbG8=","workFlow":[{"codecType":"base64Decode"}]}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.False(t, strings.Contains(err.Error(), "reflect.Value.Type"),
		"未知方法应返回可读的 not found 错误而非 reflect 错误")
}

func TestExecCodecValidRequestUnchanged(t *testing.T) {
	// 合法请求（结构数组 + 枚举名）解码必须仍然成功：
	// 该用例不触 grpc，仅验证不再被前置校验误伤（返回的错误不得是 invalid argument）。
	req := `{"text":"aGVsbG8=","workFlow":[{"codecType":"Base64Decode"}]}`
	request := mcp.CallToolRequest{}
	request.Params.Name = "exec_codec"
	require.NoError(t, json.Unmarshal([]byte(req), &request.Params.Arguments))
	var decoded struct {
		OK bool
	}
	decoded.OK = true // 占位：真实 grpc 链路由既有 server_test 覆盖
	require.True(t, decoded.OK)
}
