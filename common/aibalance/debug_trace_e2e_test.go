package aibalance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// debug_trace_e2e_test.go - DebugTraceSession 端到端集成测试.
// 触发一次真实 serveChatCompletions 请求 (mock upstream), 校验:
//   - AIBALANCE_TRACE_DIR 设置后 trace 目录被创建
//   - 5 个 trace 文件全部落盘且内容非空
//   - 03.upstream_response.raw 含上游真实回的字节 (例如 hallucinate content)
//   - 04.downstream_response.sse 含 aibalance 给客户端发的 chunked SSE 字节
//   - 上下游字节可以肉眼/grep 对比, 验证 aibalance 的转换是否丢了什么
//
// 这是用户原始诉求 "请你想办法把流量都搞出来" 的端到端验收点.
//
// 关键词: DebugTrace e2e, serveChatCompletions trace 集成, 上下游字节对比验证

func TestDebugTrace_E2E_FullRequestProducesAllFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AIBALANCE_TRACE_DIR", tmpDir)
	require.True(t, DebugTraceEnabled())

	// 上游 mock 回吐一段含 chinese-invoke tool_call 的 hallucinate content,
	// 走 react 路径 -> extractor 会把它转成 tool_calls 发给客户端.
	// 关键词: e2e mock upstream hallucinate content
	hallucinate := `[调用 bash] {"command":"echo trace-e2e"} [/tool_call]`
	srvURL, _, closeFn := rawPassthroughUpstream(t, hallucinate)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"), "expect 200 OK, got: %s", resp)

	// 校验 trace 目录: 必须有且仅有一个 session 子目录
	// (本次 chat completion 请求生成一个), 子目录名格式 ${ts}-${reqID}
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "trace_dir 必须只有 1 个 session 目录, got: %d", len(entries))
	sessionDir := filepath.Join(tmpDir, entries[0].Name())

	// 5 个 trace 文件全部落盘并验证关键内容
	checks := map[string]struct {
		mustContain    []string
		mustNotContain []string
	}{
		"00.meta.json": {
			mustContain: []string{
				`"model"`,
				`"deepseek-v4-pro"`,
				`"provider_type"`,
				`"deepseek"`,
				`"tool_mode_round1"`,
				`"react"`,
			},
		},
		"01.client_request.txt": {
			mustContain: []string{
				`"model"`,
				`"deepseek-v4-pro"`,
				`"tools"`,
				`"bash"`,
			},
		},
		"02.upstream_request.raw": {
			// 真正发给上游 wrapper 的 raw HTTP 请求: 不能含 OpenAI 协议字段
			// (react 模式下被剥掉了), 必须含 ReAct system prompt 关键词
			mustContain: []string{
				"POST",
				"HTTP/1.1",
				"deepseek-v4-pro",
			},
			mustNotContain: []string{
				`"tool_calls"`, // round2 react flatten 之后, 上游不该看到 OpenAI tool_calls 字段
				`"role":"tool"`,
				`"tools":[{`,   // tools 字段被剥掉
			},
		},
		"03.upstream_response.raw": {
			// 上游真实回的 raw HTTP response: 必须含 hallucinate 字符串
			// (即便客户端那边 extractor 已经把它转成了 tool_calls)
			mustContain: []string{
				"HTTP/1.1 200 OK",
				`[调用 bash]`,
				`echo trace-e2e`,
				`[/tool_call]`,
			},
		},
		"04.downstream_response.sse": {
			// aibalance 给客户端发的 raw bytes: 必须含 OpenAI 标准协议关键字
			// (extractor 已经把 hallucinate 文本转成 tool_calls delta)
			mustContain: []string{
				"HTTP/1.1 200 OK",
				`"tool_calls"`,
				`"bash"`,
				`echo trace-e2e`,
				"[DONE]",
			},
		},
		"05.summary.txt": {
			mustContain: []string{
				"req_id:",
				"upstream_bytes:",
				"downstream_bytes:",
			},
		},
	}
	for name, want := range checks {
		path := filepath.Join(sessionDir, name)
		raw, err := os.ReadFile(path)
		require.NoError(t, err, "trace file %s must exist", name)
		got := string(raw)
		for _, sub := range want.mustContain {
			assert.Contains(t, got, sub,
				"trace file %s must contain %q\n--- content ---\n%s", name, sub, got)
		}
		for _, sub := range want.mustNotContain {
			assert.NotContains(t, got, sub,
				"trace file %s must NOT contain %q\n--- content ---\n%s", name, sub, got)
		}
	}

	// 额外断言: meta.json 字段语义校验
	rawMeta, err := os.ReadFile(filepath.Join(sessionDir, "00.meta.json"))
	require.NoError(t, err)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(rawMeta, &meta))
	assert.Equal(t, "deepseek-v4-pro", meta["model"])
	assert.Equal(t, "deepseek", meta["provider_type"])
	assert.Equal(t, true, meta["react_extractor"], "round2 react 路径下 react_extractor 必须为 true")
	assert.Equal(t, true, meta["react_round2"], "round2 react mode 必须为 true")
}

func TestDebugTrace_E2E_DisabledEnvNoFilesCreated(t *testing.T) {
	tmpDir := t.TempDir()
	// 显式清空两个 env, 走 disabled 路径
	t.Setenv("AIBALANCE_DEBUG_TRACE", "")
	t.Setenv("AIBALANCE_TRACE_DIR", "")
	require.False(t, DebugTraceEnabled(), "env empty: trace disabled")

	srvURL, _, closeFn := rawPassthroughUpstream(t, `[调用 bash] {"command":"x"} [/tool_call]`)
	defer closeFn()

	cfg := setupReactRound2Cfg(t, srvURL, "deepseek-v4-pro")
	defer cfg.Close()

	resp := driveServeRequest(t, cfg, round2RequestBodyWithTools, nil)
	require.True(t, strings.Contains(resp, "200 OK"))

	// 既没设 AIBALANCE_TRACE_DIR, tmpDir 应该完全空
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "disabled env 时不该产生任何 trace 文件")
}
