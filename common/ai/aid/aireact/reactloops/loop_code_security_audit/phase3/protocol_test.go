package phase3

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type protocolInvoker struct{ *mock.MockInvoker }

func (*protocolInvoker) AddToTimelineWithPromptProjection(string, string, string) {}

// Fork the same way the audit dispatcher does, then execute real phase actions.
func TestAuditPhase3ForkProtocol(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			steps := []map[string]any{{"@action": "conclude_finding", "finding_id": "VULN-TEST", "status": "uncertain", "confidence": 5, "reason": "Test fixture has no source evidence."}}
			calls := 0
			callback := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				if calls >= len(steps) {
					return nil, fmt.Errorf("unexpected retry after phase completion")
				}
				args := steps[calls]
				name := args["@action"].(string)
				if native {
					delete(args, "@action")
				}
				raw, err := json.Marshal(args)
				require.NoError(t, err)
				wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
				require.Equal(t, native, wire.ToolCallCallback != nil)
				resp := cfg.NewAIResponse()
				if native {
					require.NotEmpty(t, wire.Tools)
					require.NotContains(t, req.GetPrompt(), `"@action":`)
					wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("phase3_%d", calls), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
					wire.FinishReasonCallback("tool_calls", nil)
				} else {
					require.Empty(t, wire.Tools)
					require.Contains(t, req.GetPrompt(), "@action")
					resp.EmitOutputStream(strings.NewReader(string(raw)))
				}
				calls++
				resp.Close()
				return resp, nil
			}
			cfg := aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisallowMCPServers(true), aicommon.WithDisablePerception(true),
				aicommon.WithAITransactionAutoRetry(1), aicommon.WithEnableFunctionCallMode(native),
				aicommon.WithAICallbacks(&aicommon.AICallbacks{Original: callback, QualityPriorityRaw: callback, SpeedPriorityRaw: callback}))
			parent := &protocolInvoker{mock.NewMockInvoker(ctx)}
			parent.SetConfig(cfg)
			original := aicommon.AIRuntimeInvokerGetter
			defer func() { aicommon.AIRuntimeInvokerGetter = original }()
			aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
				inv := &protocolInvoker{mock.NewMockInvoker(ctx)}
				inv.SetConfig(aicommon.NewConfig(ctx, opts...))
				return inv, nil
			}
			parentTask := aicommon.NewStatefulTaskBase("audit-parent", "audit", ctx, cfg.GetEmitter(), true)
			child, task, _, release, err := reactloops.PrepareForkedSubAgent(parent, parentTask, reactloops.SubAgentJob{Identifier: "phase3", Goal: "audit protocol fixture"})
			require.NoError(t, err)
			defer release()
			require.Equal(t, native, child.GetConfig().GetConfigBool("EnableFunctionCallMode"))
			state := model.NewAuditState()
			state.WorkDir = t.TempDir()
			state.ProjectPath = t.TempDir()
			finding := &model.Finding{ID: "VULN-TEST", File: "entry.go", Title: "Test finding"}
			state.AddFinding(finding)
			loop, err := buildSingleFindingVerifyLoop(child, state, finding, 1, 1)
			require.NoError(t, err)
			defer loop.Release()
			require.Equal(t, native, loop.FunctionCallModeEnabled())
			var toolNames []string
			for _, tool := range buildinaitools.GetAllTools() {
				toolNames = append(toolNames, tool.GetName())
			}
			require.NoError(t, reactloops.ValidateActionToolNames(loop.GetAllActions(), toolNames))
			require.NoError(t, loop.ExecuteWithExistedTask(task))
			require.Equal(t, len(steps), calls)
			require.Equal(t, 1, state.GetVerifiedVulnCount())
		})
	}
}
