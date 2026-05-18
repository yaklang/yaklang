package aibalance

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// debug_trace_test.go - DebugTraceSession 单元测试.
// 覆盖:
//   - env 未设置时 NewDebugTraceSession 返回 nil (no-op 零开销 fallback)
//   - env 设置 AIBALANCE_TRACE_DIR 时正常落盘 5 个文件, 文件名固定
//   - WriteMeta / WriteClientRequest / WriteUpstreamRequestResponse / dumpConn.Write
//     四条写路径都能正确落字节
//   - Close 写 summary, 幂等
//   - WrapConnForTrace(conn, nil) 零开销返回原 conn
//   - dumpConn.Write 同步 tee 到 trace 文件 + 真实底层 conn
//
// 关键词: DebugTraceSession unit test, env 开关, nil-safe receiver, dumpConn tee

func TestDebugTrace_DisabledEnv_ReturnsNil(t *testing.T) {
	// 清空 env, 确保 NewDebugTraceSession 走 disabled 分支
	t.Setenv("AIBALANCE_DEBUG_TRACE", "")
	t.Setenv("AIBALANCE_TRACE_DIR", "")
	require.False(t, DebugTraceEnabled(), "without env, DebugTraceEnabled should be false")

	s := NewDebugTraceSession("test-req-1")
	assert.Nil(t, s, "disabled env: NewDebugTraceSession must return nil")

	// nil-safe receiver: 所有方法都不能 panic
	s.WriteMeta(map[string]any{"x": 1})
	s.WriteClientRequest([]byte("hello"))
	s.WriteUpstreamRequestResponse([]byte("req"), []byte("hdr"), []byte("body"), nil)
	s.Close()
	assert.Equal(t, "", s.ReqID())
	assert.Equal(t, "", s.RootDir())
}

func TestDebugTrace_EnabledEnv_WritesAllFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AIBALANCE_TRACE_DIR", tmpDir)
	require.True(t, DebugTraceEnabled(), "with AIBALANCE_TRACE_DIR, DebugTraceEnabled should be true")
	assert.Equal(t, tmpDir, DebugTraceRootDir(), "DebugTraceRootDir should reflect env")

	s := NewDebugTraceSession("test-req-2")
	require.NotNil(t, s, "enabled env: NewDebugTraceSession must return non-nil")
	require.Equal(t, "test-req-2", s.ReqID())
	require.Contains(t, s.RootDir(), tmpDir, "root_dir should be under AIBALANCE_TRACE_DIR")

	s.WriteMeta(map[string]any{"model": "deepseek-v4-pro", "provider": "deepseek"})
	s.WriteClientRequest([]byte(`{"model":"deepseek-v4-pro","stream":true}`))
	s.WriteUpstreamRequestResponse(
		[]byte("POST /v1/chat/completions HTTP/1.1\r\nHost: api.deepseek.com\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"),
		&aispec.ChatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	)

	s.Close()

	// 校验 5 个落盘文件都存在且非空
	root := s.RootDir()
	expected := map[string]string{
		"00.meta.json":               `"model"`,
		"01.client_request.txt":      `deepseek-v4-pro`,
		"02.upstream_request.raw":    `POST /v1/chat/completions`,
		"03.upstream_response.raw":   `HTTP/1.1 200 OK`,
		"04.downstream_response.sse": "",
		"05.summary.txt":             "upstream_bytes",
	}
	for name, substr := range expected {
		path := filepath.Join(root, name)
		st, err := os.Stat(path)
		require.NoError(t, err, "trace file %s should exist", name)
		assert.GreaterOrEqual(t, st.Size(), int64(0), "trace file %s should be non-negative size", name)
		if substr != "" {
			raw, err := os.ReadFile(path)
			require.NoError(t, err, "read trace file %s", name)
			assert.Contains(t, string(raw), substr, "trace file %s content sanity", name)
		}
	}
}

func TestDebugTrace_Close_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AIBALANCE_TRACE_DIR", tmpDir)

	s := NewDebugTraceSession("idem")
	require.NotNil(t, s)
	s.Close()

	// 第二次 Close 不能 panic, 不能重复写 summary
	assert.NotPanics(t, func() { s.Close() })

	root := s.RootDir()
	st, err := os.Stat(filepath.Join(root, "05.summary.txt"))
	require.NoError(t, err)
	firstModTime := st.ModTime()

	// 等 10ms 再 Close, summary mtime 不能变 (没被重写)
	time.Sleep(10 * time.Millisecond)
	s.Close()
	st2, err := os.Stat(filepath.Join(root, "05.summary.txt"))
	require.NoError(t, err)
	assert.Equal(t, firstModTime, st2.ModTime(), "Close 必须幂等, summary 不能被二次重写")
}

func TestDebugTrace_WrapConnForTrace_NilFallback(t *testing.T) {
	// trace=nil 时 WrapConnForTrace 必须零开销返回原 conn (相同指针)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	wrapped := WrapConnForTrace(server, nil)
	assert.Same(t, server, wrapped, "trace=nil: WrapConnForTrace must return original conn (no allocation)")
}

func TestDebugTrace_DumpConn_TeesWritesToTraceFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AIBALANCE_TRACE_DIR", tmpDir)

	s := NewDebugTraceSession("tee-test")
	require.NotNil(t, s)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	wrapped := WrapConnForTrace(server, s)
	require.NotSame(t, server, wrapped, "trace!=nil: WrapConnForTrace must return *dumpConn (different pointer)")

	// 客户端协程读 server (经 wrap) 写入的字节
	gotByClient := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := client.Read(buf)
		gotByClient <- buf[:n]
	}()

	payload := []byte("HTTP/1.1 200 OK\r\n\r\ndata: hello\n\n")
	_, err := wrapped.Write(payload)
	require.NoError(t, err)

	// 真实底层 conn 必须收到完整字节
	select {
	case got := <-gotByClient:
		assert.Equal(t, payload, got, "dumpConn.Write 必须把原字节透传给真实底层 conn")
	case <-time.After(2 * time.Second):
		t.Fatal("client 端 1s 内没读到 wrapped.Write 的字节")
	}

	s.Close()

	// trace 文件 04.downstream_response.sse 必须包含完整字节
	rawDown, err := os.ReadFile(filepath.Join(s.RootDir(), "04.downstream_response.sse"))
	require.NoError(t, err)
	assert.Equal(t, payload, rawDown, "dumpConn.Write 必须同步 tee 到 04.downstream_response.sse")
}

func TestDebugTrace_DebugTraceEnabled_BothEnvs(t *testing.T) {
	t.Setenv("AIBALANCE_DEBUG_TRACE", "")
	t.Setenv("AIBALANCE_TRACE_DIR", "")
	assert.False(t, DebugTraceEnabled(), "both empty: disabled")

	t.Setenv("AIBALANCE_DEBUG_TRACE", "1")
	assert.True(t, DebugTraceEnabled(), "AIBALANCE_DEBUG_TRACE=1: enabled")

	t.Setenv("AIBALANCE_DEBUG_TRACE", "")
	t.Setenv("AIBALANCE_TRACE_DIR", "/tmp/anywhere")
	assert.True(t, DebugTraceEnabled(), "AIBALANCE_TRACE_DIR set: enabled")
}

func TestDebugTrace_MetaJSONShape(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AIBALANCE_TRACE_DIR", tmpDir)

	s := NewDebugTraceSession("meta-shape")
	require.NotNil(t, s)
	meta := map[string]any{
		"req_id":   "meta-shape",
		"model":    "deepseek-v4-pro",
		"provider": "deepseek",
		"tools":    3,
		"react":    true,
	}
	s.WriteMeta(meta)
	s.Close()

	raw, err := os.ReadFile(filepath.Join(s.RootDir(), "00.meta.json"))
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded), "00.meta.json 必须是 valid JSON")
	assert.Equal(t, "deepseek-v4-pro", decoded["model"])
	assert.Equal(t, true, decoded["react"])
}

func TestDebugTrace_DebugTraceCallbackForAispec_NilTraceReturnsNil(t *testing.T) {
	t.Setenv("AIBALANCE_DEBUG_TRACE", "")
	t.Setenv("AIBALANCE_TRACE_DIR", "")
	assert.Nil(t, debugTraceCallbackForAispec(nil),
		"trace=nil: debugTraceCallbackForAispec 必须返回 nil 让 aispec 跳过 append")
}

func TestDebugTrace_DebugTraceCallbackForAispec_WritesUpstream(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AIBALANCE_TRACE_DIR", tmpDir)

	s := NewDebugTraceSession("cb")
	require.NotNil(t, s)
	cb := debugTraceCallbackForAispec(s)
	require.NotNil(t, cb)

	cb(
		[]byte("POST /v1 HTTP/1.1\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\n\r\n"),
		[]byte("data: {}\n\ndata: [DONE]\n\n"),
		&aispec.ChatUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	)
	s.Close()

	rawReq, err := os.ReadFile(filepath.Join(s.RootDir(), "02.upstream_request.raw"))
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(rawReq, []byte("POST /v1 HTTP/1.1")),
		"02.upstream_request.raw 起始必须是 POST 请求行")

	rawResp, err := os.ReadFile(filepath.Join(s.RootDir(), "03.upstream_response.raw"))
	require.NoError(t, err)
	assert.Contains(t, string(rawResp), "HTTP/1.1 200 OK")
	assert.Contains(t, string(rawResp), "data: [DONE]")
}
