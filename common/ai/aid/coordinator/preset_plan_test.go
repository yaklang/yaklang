package coordinator_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
)

func TestCoordinatorPresetPlanApprovalContext(t *testing.T) {
	for _, mocker := range []bool{false, true} {
		name := "preset"
		if mocker {
			name = "mocker"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			root := &coordinator.PlanNode{Name: "来源核对", Goal: "核对来源后形成报告", Identifier: "sources", Subtasks: []*coordinator.PlanNode{
				{Name: "读取来源", Goal: "读取 source.txt 并保存实际证据", Identifier: "read_source"},
				{Name: "复核来源", Goal: "根据前置证据复核来源", Identifier: "verify_source", DependsOn: []string{"read_source"}},
			}}
			raw, err := json.Marshal(root)
			require.NoError(t, err)
			option := coordinator.WithPresetPlan(string(raw), "# 核对方案\n只读检查来源，保存证据，交付报告。")
			mockerCalls := 0
			if mocker {
				option = coordinator.WithPlanMocker(func(*coordinator.Session) *coordinator.PlanResponse {
					mockerCalls++
					return &coordinator.PlanResponse{RootTask: root, Document: "# 核对方案\n只读检查来源，保存证据，交付报告。"}
				})
			}
			var prompts []string
			s, err := coordinator.NewSession(ctx, "请核对 source.txt，计划确认一次后执行。",
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithEnableFunctionCallMode(true), aicommon.WithGenerateReport(false), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAgreeYOLO(), option,
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					prompts = append(prompts, req.GetPrompt())
					switch len(prompts) {
					case 1:
						return nativeResponse(c, req, "submit_plan", map[string]any{})
					default:
						return nativeResponse(c, req, "finish", map[string]any{})
					}
				}))
			require.NoError(t, err)
			defer s.Close()
			_, err = s.AppendUserInputHistory("已有约束：只读，不访问外部网络。", time.Now())
			require.NoError(t, err)
			s.Timeline.PushText(s.AcquireId(), "环境检查：source.txt 位于当前工作区。")
			evidence, err := aicommon.BuildSessionEvidenceUpsert("source.location", "主体：source.txt；动作：检查工作区；观测：文件位于当前工作区；控制含义：后续任务可在本地读取。")
			require.NoError(t, err)
			s.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{evidence})
			s.Timeline.FreezeAll()
			require.NoError(t, s.RunPlanOnly())
			require.Len(t, prompts, 2)
			if mocker {
				require.Equal(t, 1, mockerCalls)
			}
			for _, prompt := range prompts {
				sections := aiprojection.Project(aiprojection.ProjectionInput{Prompt: prompt}).Messages
				require.NotEmpty(t, sections)
				require.Contains(t, prompt, "你是计划协调员")
				require.Contains(t, prompt, "PLAN DEFINITION")
				require.Contains(t, prompt, "读取 source.txt 并保存实际证据")
				require.Contains(t, prompt, "source.location")
				require.Equal(t, 1, strings.Count(prompt, "请核对 source.txt，计划确认一次后执行。"))
				require.NotContains(t, prompt, "You coordinate a plan")
				dynamic := strings.Split(prompt, "<|PROMPT_SECTION_dynamic_")[1]
				require.NotContains(t, dynamic, "请核对 source.txt")
				require.NotContains(t, dynamic, "已有约束：")
			}
			require.Contains(t, prompts[0], "阶段：PLAN；已有计划：true")
			require.NotContains(t, prompts[1], `"draft":{`, "actions must not replay the complete plan")
			require.NotContains(t, prompts[1], "(*coordinator.Plan)")
			require.Contains(t, prompts[1], "# PLAN DOCUMENT")
			require.Contains(t, prompts[1], "Dispatch: blocked")
			if dir := os.Getenv("COORDINATOR_CONTEXT_REVIEW_DIR"); dir != "" && !mocker {
				require.NoError(t, os.MkdirAll(dir, 0755))
				for i, filename := range []string{"01-draft.txt", "03-approved.txt"} {
					require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(prompts[i]), 0600))
				}
			}
		})
	}
}
