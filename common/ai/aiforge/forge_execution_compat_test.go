package aiforge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestForgeResultAliasesAndOpenBusinessData(t *testing.T) {
	for _, native := range []bool{false, true} {
		builder := NewYakForgeBlueprintConfig("aliases", "Analyze data", "").WithResultPrompt("Return business data").WithActionName("result, old_result")
		blueprint, err := builder.Build()
		require.NoError(t, err)
		execution := newForgeResultTestCoordinator(t, blueprint, func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
			const data = `{"@action":"old_result","nested":{"values":[1,"two",null],"ok":true}}`
			response := c.NewAIResponse()
			if native {
				require.NotNil(t, wire.ToolCallCallback)
				wire.ToolCallCallback([]*aispec.ToolCall{{ID: "alias-output", Type: "function", Function: aispec.FuncReturn{Name: "result", Arguments: data}}})
				wire.FinishReasonCallback("tool_calls", nil)
			} else {
				require.Empty(t, wire.Tools)
				response.EmitOutputStream(strings.NewReader(data))
			}
			response.Close()
			return response, nil
		})
		execution.EnableFunctionCallMode = native
		_, err = execution.deliver(execution.GetContext(), execution.Session, execution.Snapshot())
		require.NoError(t, err)
		result := execution.Result()
		require.Equal(t, "result", result.Action.Name())
		require.True(t, result.Action.GetInvokeParams("nested").GetBool("ok"))
		var output map[string]any
		require.NoError(t, json.Unmarshal([]byte(result.Formated.(string)), &output))
		require.Equal(t, []any{float64(1), "two", nil}, output["nested"].(map[string]any)["values"])
	}
}

func TestForgeValidatedResultRetainsInvocationWithoutParsingImportedCLI(t *testing.T) {
	registerFormattingRuntime(t)
	blueprint := &ForgeBlueprint{Name: "validated-result", InitializePrompt: "Prepare result", ResultPrompt: `{{.Forge.UserQuery}} {{.Forge.UserParams}} {{.Memory.OS}} {{.ContextProvider.Arch}}`, ParameterRuleYaklangCode: `panic("must not parse or execute")`}
	blueprint.ResultGenerator = func(e *ForgeExecution, prompt string) (string, error) {
		require.Contains(t, prompt, "caller query")
		require.Contains(t, prompt, "verified-input.txt")
		require.Contains(t, prompt, e.GetContextProvider().OS())
		require.Contains(t, prompt, e.GetContextProvider().Arch())
		require.NotContains(t, prompt, "<execution_material>", "custom generators own context composition")
		return "formatted", nil
	}
	execution, err := blueprint.CreateCoordinatorWithQueryAndParams(context.Background(), "caller query", []Parameter{{Key: "file", Value: "verified-input.txt"}}, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true))
	require.NoError(t, err)
	t.Cleanup(execution.Close)
	_, err = execution.deliver(execution.GetContext(), execution.Session, execution.Snapshot())
	require.NoError(t, err)
	require.Equal(t, "formatted", execution.Result().Formated)
}
