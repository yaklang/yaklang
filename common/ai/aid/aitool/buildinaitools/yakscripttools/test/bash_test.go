package test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

func getBashTool(t *testing.T) *aitool.Tool {
	t.Helper()
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/system/bash.yak")
	assert.NilError(t, err)
	aiTool := yakscripttools.LoadYakScriptToAiTools("bash", string(content))
	assert.Assert(t, aiTool != nil, "failed to parse bash.yak metadata")
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	assert.Assert(t, len(tools) == 1, "ConvertTools returned %d tools", len(tools))
	return tools[0]
}

func invokeBashResult(t *testing.T, params aitool.InvokeParams) map[string]any {
	t.Helper()
	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	result, err := getBashTool(t).Callback(context.Background(), params, nil, stdout, stderr)
	assert.NilError(t, err)
	semantic := utils.InterfaceToGeneralMap(result)
	assert.Assert(t, len(semantic) > 0, "missing semantic RESULT; stdout=%s stderr=%s", stdout.String(), stderr.String())
	return semantic
}

func TestBashToolReportsObjectiveExitSemantics(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		accepted   string
		exitCode   int
		isAccepted bool
	}{
		{name: "stdout_claims_success_but_exit_7", command: "printf 'SUCCESS\\n'; exit 7", exitCode: 7},
		{name: "stderr_claims_error_but_exit_0", command: "printf 'ERROR\\n' >&2; exit 0", exitCode: 0, isAccepted: true},
		{name: "earlier_false_but_final_exit_0", command: "false; echo complete", exitCode: 0, isAccepted: true},
		{name: "pipeline_without_pipefail_uses_last_exit", command: "false | true", exitCode: 0, isAccepted: true},
		{name: "pipeline_with_pipefail_exposes_failure", command: "set -o pipefail; false | true", exitCode: 1},
		{name: "explicitly_accept_exit_1", command: "exit 1", accepted: "0,1", exitCode: 1, isAccepted: true},
		{name: "normalize_accepted_exit_code", command: "exit 1", accepted: "0,01", exitCode: 1, isAccepted: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := aitool.InvokeParams{"command": tc.command, "shell": "bash", "timeout": 5}
			if tc.accepted != "" {
				params["accepted-exit-codes"] = tc.accepted
			}
			result := invokeBashResult(t, params)
			assert.Equal(t, utils.InterfaceToInt(result["exit_code"]), tc.exitCode)
			assert.Equal(t, utils.InterfaceToBoolean(result["exit_code_available"]), true)
			assert.Equal(t, utils.InterfaceToBoolean(result["exit_code_accepted"]), tc.isAccepted)
			assert.Equal(t, utils.InterfaceToBoolean(result["timed_out"]), false)
		})
	}
}

func TestBashToolNonZeroExitStillCompletesInvocationProtocol(t *testing.T) {
	tool := getBashTool(t)
	result, err := tool.InvokeWithParams(aitool.InvokeParams{
		"command": "printf 'SUCCESS\\n'; exit 7",
		"shell":   "bash",
		"timeout": 5,
	})
	assert.NilError(t, err)
	assert.Equal(t, result.Success, true)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	assert.Assert(t, ok)
	semantic := utils.InterfaceToGeneralMap(execution.Result)
	assert.Equal(t, utils.InterfaceToInt(semantic["exit_code"]), 7)
	assert.Equal(t, utils.InterfaceToBoolean(semantic["exit_code_accepted"]), false)
	timeline := result.String()
	assert.Assert(t, !strings.Contains(timeline, "success: true"))
	assert.Assert(t, !strings.Contains(timeline, "tool/bash ok"))
	assert.Assert(t, strings.Contains(timeline, "exit_code"))
}

func TestBashToolParameterValidationIsProtocolFailure(t *testing.T) {
	result, err := getBashTool(t).InvokeWithParams(aitool.InvokeParams{"timeout": 5})
	assert.ErrorContains(t, err, "参数验证失败")
	assert.Equal(t, result.Success, false)
	assert.Assert(t, strings.Contains(result.String(), "protocol_error"))
}

func TestBashToolAcceptedExitCodesCompatibilityAliasIsTyped(t *testing.T) {
	tool := getBashTool(t)

	result, err := tool.InvokeWithParams(aitool.InvokeParams{
		"command":             "exit 1",
		"shell":               "bash",
		"timeout":             5,
		"accepted_exit_codes": "0,1",
	})
	assert.NilError(t, err)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	assert.Assert(t, ok)
	semantic := utils.InterfaceToGeneralMap(execution.Result)
	assert.Equal(t, utils.InterfaceToInt(semantic["exit_code"]), 1)
	assert.Equal(t, utils.InterfaceToBoolean(semantic["exit_code_accepted"]), true)

	invalid, invalidErr := tool.InvokeWithParams(aitool.InvokeParams{
		"command":             "exit 1",
		"shell":               "bash",
		"timeout":             5,
		"accepted_exit_codes": []any{0, 1},
	})
	assert.ErrorContains(t, invalidErr, "参数验证失败")
	assert.Equal(t, invalid.Success, false)
}

func TestBashToolTimeoutHasNoFabricatedExitCode(t *testing.T) {
	result := invokeBashResult(t, aitool.InvokeParams{"command": "sleep 2", "shell": "bash", "timeout": 1})
	assert.Equal(t, utils.InterfaceToBoolean(result["timed_out"]), true)
	assert.Equal(t, utils.InterfaceToBoolean(result["exit_code_available"]), false)
	assert.Assert(t, utils.IsNil(result["exit_code"]), "timeout must expose a null exit code, got %#v", result["exit_code"])
	assert.Equal(t, utils.InterfaceToString(result["termination_reason"]), "timeout")
}

func TestBashToolNormalizesGeneratedScriptLineEndings(t *testing.T) {
	tool := getBashTool(t)
	stdout, stderr := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), aitool.InvokeParams{
		"command": "printf 'BASH_CRLF_OK\\n'",
		"shell":   "bash",
		"timeout": 10,
	}, nil, stdout, stderr)

	assert.NilError(t, err)
	combined := stdout.String() + stderr.String()
	assert.Assert(t, strings.Contains(combined, "BASH_CRLF_OK"), "command output missing: %s", combined)
	assert.Assert(t, !strings.Contains(combined, "command not found"), "generated script still contains CRLF: %s", combined)
	assert.Assert(t, !strings.Contains(combined, "exit code 127"), "bash tool unexpectedly failed: %s", combined)
}
