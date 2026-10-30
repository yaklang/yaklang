package phase4_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/phase4"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestAuditReportDelegationProtocolAndFailure(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%t/fail=%t", native, fail), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				calls := 0
				runtime, err := aireact.NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						calls++
						wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						// Initialization failure may request separate natural-language
						// advice. The report decision requests keep the session protocol.
						if !fail || calls == 1 {
							require.Equal(t, native, wire.ToolCallCallback != nil)
						}
						if fail {
							return nil, fmt.Errorf("report provider unavailable")
						}
						name := "write_section"
						args := map[string]any{}
						if calls == 1 {
							args["content"] = "# 完整报告\n\n测试项目未发现漏洞。\n"
						} else {
							name = "finish"
						}
						resp := c.NewAIResponse()
						if native {
							raw, _ := json.Marshal(args)
							wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("report_%d", calls), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
							wire.FinishReasonCallback("tool_calls", nil)
						} else {
							require.Empty(t, wire.Tools)
							body, _ := args["content"].(string)
							delete(args, "content")
							args["@action"] = name
							raw, _ := json.Marshal(args)
							text := string(raw)
							if body != "" {
								text += "\n<|GEN_REPORT_CURRENT_NONCE|>\n" + body + "\n<|GEN_REPORT_END_CURRENT_NONCE|>"
							}
							resp.EmitOutputStream(strings.NewReader(text))
						}
						resp.Close()
						return resp, nil
					}))
				require.NoError(t, err)
				state := model.NewAuditState()
				state.WorkDir = t.TempDir()
				state.ProjectPath = t.TempDir()
				loop, err := phase4.BuildReportLoop(runtime, state)
				require.NoError(t, err)
				defer loop.Release()
				task := aicommon.NewStatefulTaskBase("audit-report", "Generate report", ctx, runtime.GetConfig().GetEmitter(), true)
				err = loop.ExecuteWithExistedTask(task)
				timeline := runtime.GetConfig().(*aicommon.Config).GetTimeline().Dump()
				if fail {
					require.ErrorContains(t, err, "report provider unavailable")
					require.NotContains(t, timeline, "PHASE4_COMPLETE")
				} else {
					require.NoError(t, err)
					require.Equal(t, 2, calls)
					require.Contains(t, timeline, "PHASE4_COMPLETE")
					written, err := os.ReadFile(state.GetFinalReportPath())
					require.NoError(t, err)
					require.Contains(t, string(written), "# 完整报告")
				}
			})
		}
	}
}
