package aitool

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestToolExecutionResultJSONKeepsAllSemanticChannels(t *testing.T) {
	raw, err := json.Marshal(&ToolExecutionResult{})
	require.NoError(t, err)
	require.JSONEq(t, `{"stdout":"","stderr":"","combined_output":"","result":null}`, string(raw))
}

func TestToolResultDumpAndTimelineUseNeutralProtocolSemantics(t *testing.T) {
	tr := &ToolResult{
		Name:    "bash",
		Success: true,
		Data: &ToolExecutionResult{
			Stdout:         "SUCCESS from stdout",
			Stderr:         "ERROR from stderr",
			CombinedOutput: "SUCCESS from stdout\nERROR from stderr",
			Result: map[string]any{
				"exit_code":          7,
				"exit_code_accepted": false,
			},
		},
	}

	dump := tr.Dump()
	require.Contains(t, dump, `"protocol_completed":true`)
	require.Contains(t, dump, `"execution_status":"failed"`)
	require.NotContains(t, dump, `"success"`)
	require.Contains(t, dump, `"exit_code":7`)

	timeline := tr.String()
	require.Contains(t, timeline, "execution_status: failed")
	require.Less(t, strings.Index(timeline, "execution_result:"), strings.Index(timeline, "observations:"))
	require.NotContains(t, timeline, "success: true")
	require.NotContains(t, timeline, "tool/bash ok")

	failed := (&ToolResult{Name: "bash", Success: false, Error: "validation failed"}).String()
	require.Contains(t, failed, "protocol_error: validation failed")
	require.NotContains(t, failed, "execution failed")
}

func TestToolResultGetExecutionStatusUsesOnlyExplicitSignals(t *testing.T) {
	tests := []struct {
		name       string
		result     any
		wantStatus ToolExecutionStatus
		wantDetail string
	}{
		{
			name:       "accepted zero exit",
			result:     map[string]any{"exit_code": 0, "exit_code_accepted": true, "timed_out": false},
			wantStatus: ToolExecutionStatusSucceeded,
			wantDetail: "exit_code_accepted=true",
		},
		{
			name:       "rejected nonzero exit",
			result:     map[string]any{"exit_code": 7, "exit_code_accepted": false, "process_error": "exit status 7"},
			wantStatus: ToolExecutionStatusFailed,
			wantDetail: "exit_code=7",
		},
		{
			name:       "accepted nonzero exit remains success",
			result:     map[string]any{"exit_code": 7, "exit_code_accepted": true, "process_error": "exit status 7"},
			wantStatus: ToolExecutionStatusSucceeded,
			wantDetail: "exit_code_accepted=true",
		},
		{
			name:       "timeout overrides exit acceptance",
			result:     map[string]any{"exit_code": 0, "exit_code_accepted": true, "timed_out": true},
			wantStatus: ToolExecutionStatusFailed,
			wantDetail: "timed_out=true",
		},
		{
			name:       "transport error",
			result:     map[string]any{"transport_error": "connection refused", "response_received": false},
			wantStatus: ToolExecutionStatusFailed,
			wantDetail: "transport_error=connection refused",
		},
		{
			name:       "http error response is task dependent",
			result:     map[string]any{"status_code": 500, "response_received": true, "transport_error": ""},
			wantStatus: ToolExecutionStatusUnknown,
		},
		{
			name:       "arbitrary domain result is task dependent",
			result:     map[string]any{"matches": 3},
			wantStatus: ToolExecutionStatusUnknown,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			toolResult := &ToolResult{
				Name:    "test-tool",
				Success: true,
				Data:    &ToolExecutionResult{Result: test.result},
			}
			status, detail := toolResult.GetExecutionStatus()
			require.Equal(t, test.wantStatus, status)
			if test.wantDetail != "" {
				require.Contains(t, detail, test.wantDetail)
			}
		})
	}
}

func TestToolResultShrinkCannotHideExplicitExecutionFailure(t *testing.T) {
	toolResult := &ToolResult{
		Name:         "bash",
		Success:      true,
		ShrinkResult: "The command was processed.",
		Data: &ToolExecutionResult{Result: map[string]any{
			"exit_code":          9,
			"exit_code_accepted": false,
		}},
	}

	timeline := toolResult.String()
	require.Contains(t, timeline, "execution_status: failed")
	require.Contains(t, timeline, "The command was processed.")
	require.Less(t, strings.Index(timeline, "execution_status: failed"), strings.Index(timeline, "The command was processed."))
}

func TestToolResult_DumpTimelineItem_ParamsOption(t *testing.T) {
	tr := &ToolResult{
		Name:  "direct_tool",
		Param: map[string]any{"secret_marker": "only-once"},
		Data:  map[string]any{"result": "ok"},
	}

	withParams := bytes.NewBuffer(nil)
	tr.DumpTimelineItem(withParams)
	require.Contains(t, withParams.String(), "param:")
	require.Contains(t, withParams.String(), "secret_marker")

	withoutParams := bytes.NewBuffer(nil)
	tr.DumpTimelineItem(withoutParams, WithToolResultDumpParams(false))
	require.NotContains(t, withoutParams.String(), "param:")
	require.NotContains(t, withoutParams.String(), "secret_marker")
	require.Contains(t, withoutParams.String(), "result:")

	tr.OmitParamsInTimeline = true
	require.Equal(t, withoutParams.String(), tr.String())
}

// TestToolResult_String_YAMLParamFlushLeft 验证 ToolResult.String() 的 yaml param
// 块顶层 key 顶头 (无外层 "  " 缩进), 节省 prompt token; yaml 自身的 block scalar
// 相对缩进保留, 仍可被 yaml.Unmarshal 正确解析.
//
// 关键词: ToolResult.String yaml 顶头, timeline render dedent
func TestToolResult_String_YAMLParamFlushLeft(t *testing.T) {
	tr := &ToolResult{
		Name: "bash",
		Param: map[string]any{
			"command": "echo hi\necho ok",
			"timeout": 20,
		},
		Success: true,
	}

	out := tr.String()
	t.Logf("ToolResult.String:\n%s", out)

	require.Contains(t, out, "param:\n",
		"param header must be flush left with no trailing space, got: %q", out)

	// 顶层 yaml key 顶头: 不应再出现 "  command:" / "  timeout:" 这种带 2 空格前缀.
	require.NotContains(t, out, "\n  command:",
		"top-level yaml key 'command' should sit flush left, got: %q", out)
	require.NotContains(t, out, "\n  timeout:",
		"top-level yaml key 'timeout' should sit flush left, got: %q", out)

	// command 顶头出现
	require.Contains(t, out, "\ncommand: |-\n",
		"command should appear as top-level yaml block scalar, got: %q", out)

	// command body 行 (yaml block scalar 自身缩 2 空格 indent_2 模式) — 不应出现 6 空格
	// 历史上 echo 行因 yaml 4 + 外套 2 出现 6 空格; 现在只剩 yaml 自带缩进.
	require.NotContains(t, out, "      echo hi",
		"echo body should not carry historical 6-space indent (yaml 4 + outer 2), got: %q", out)
	require.NotContains(t, out, "      echo ok",
		"echo body should not carry historical 6-space indent, got: %q", out)

	// 验证 yaml block 仍可被 yaml.Unmarshal 正确解析: 抽出 "param:\n" 之后的 yaml 段,
	// 取第一段直到下一个不以 yaml 字符开头的行 (本测试简单切到 success/data/error 等
	// 字段或末尾).
	idx := strings.Index(out, "param:\n")
	require.GreaterOrEqual(t, idx, 0)
	yamlBlock := out[idx+len("param:\n"):]
	// 切到下一个 ToolResult 顶层字段, 比如 "success", "data", "error", "shrink_*"
	for _, sep := range []string{"\nsuccess:", "\ndata:", "\nerror:", "\nshrink_"} {
		if k := strings.Index(yamlBlock, sep); k >= 0 {
			yamlBlock = yamlBlock[:k]
		}
	}

	parsed := map[string]any{}
	err := yaml.Unmarshal([]byte(yamlBlock), &parsed)
	require.NoErrorf(t, err, "yaml block must remain parseable, got block:\n%s", yamlBlock)
	require.Equal(t, "echo hi\necho ok", parsed["command"], "yaml command should round-trip exactly")
	require.EqualValues(t, 20, parsed["timeout"])
}

// TestToolResult_String_EmptyParam 验证空 param 走 jsonify 分支保持不变.
//
// 关键词: ToolResult.String 空参数 jsonify
func TestToolResult_String_EmptyParam(t *testing.T) {
	tr := &ToolResult{
		Name:    "noop",
		Param:   nil,
		Success: true,
	}
	out := tr.String()
	t.Logf("empty param ToolResult.String:\n%s", out)
	require.Contains(t, out, "param: ",
		"empty param branch should fall through to jsonify, got: %q", out)
	require.NotContains(t, out, "param:\n", "empty param branch should NOT take yaml path")
}
