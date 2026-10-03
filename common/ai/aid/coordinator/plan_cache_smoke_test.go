package coordinator_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// Compare the provider-facing stable messages, not raw prompt tags/nonces.
// Large-plan queries must change only the open tail; document/approval changes
// may update SemiDynamic1, but must never invalidate static rules/tool schemas.
func TestCoordinatorPlanCacheStableAcrossRepeatedRequests(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call_%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var tasks []any
			for i := 0; i < 32; i++ {
				tasks = append(tasks, map[string]any{"subtask_name": fmt.Sprintf("核对 %d", i), "subtask_goal": fmt.Sprintf("任务 %d 的冻结任务书：", i) + strings.Repeat("核对真实来源并保留验收依据。", 16), "subtask_identifier": fmt.Sprintf("source_%d", i), "depends_on": []string{}})
			}
			data, err := json.Marshal(map[string]any{"main_task": "大计划缓存验证", "main_task_goal": "反复核对当前计划且不复制计划内容", "tasks": tasks})
			require.NoError(t, err)
			const documentSentinel = "immutable-plan-document-sentinel"
			document := documentSentinel + "\n" + strings.Repeat("原始计划文档：约束、来源、验收标准不随查询改变。\n", 512)
			calls := 0
			var s *coordinator.Session
			stable := map[string][4][32]byte{}
			phaseCalls := map[string]int{}
			var permanent [2][32]byte // High Static and Frozen remain fixed across the phase handoff.
			var toolsHash [32]byte
			var ratios []float64
			s, err = coordinator.NewSession(ctx, "核对预设大计划并提交，批准一次后结束本次规划。",
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
				aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(false),
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(native), aicommon.WithAgreeYOLO(),
				coordinator.WithPresetPlan(string(data), document),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					require.Equal(t, "react-loop:coordinator", req.GetCallerLabel())
					if calls >= 18 {
						return nil, fmt.Errorf("requests did not converge")
					}
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					projected := aiprojection.Project(aiprojection.ProjectionInput{Prompt: req.GetPrompt(), ActionTools: wire.Tools})
					require.True(t, projected.Metadata.CacheProjected)
					require.GreaterOrEqual(t, len(projected.Messages), 5, "do not collapse the five cache regions")
					var prefix [4][32]byte
					prefixBytes := 0
					for i := range prefix {
						msg, err := json.Marshal(projected.Messages[i])
						require.NoError(t, err)
						prefix[i] = sha256.Sum256(msg)
						prefixBytes += len(msg)
					}
					declaredTools, err := json.Marshal(projected.Tools)
					require.NoError(t, err)
					fixed := [2][32]byte{prefix[0], prefix[1]}
					if calls == 0 {
						permanent = fixed
					} else {
						require.Equal(t, permanent, fixed, "query or approval must not rewrite High Static/Frozen")

					}
					state := s.Snapshot()
					phase := string(state.Phase)
					if phaseCalls[phase] > 0 {
						require.Equal(t, toolsHash, sha256.Sum256(declaredTools), "tool contracts stay stable within phase")
					}
					toolsHash = sha256.Sum256(declaredTools)
					if previous, ok := stable[phase]; ok {
						require.Equal(t, previous, prefix, "ordinary requests must preserve the complete stable prefix")
						messages, err := json.Marshal(projected.Messages)
						require.NoError(t, err)
						// Native tool declarations are also a stable part of the
						// provider request. Count both messages and tools consistently.
						ratio := float64(prefixBytes+len(declaredTools)) / float64(len(messages)+len(declaredTools))
						require.Greater(t, ratio, 0.85, "stable messages and tools must dominate even with a large plan")
						ratios = append(ratios, ratio)
					}
					stable[phase] = prefix
					phaseCalls[phase]++
					// Planning-only feedback is the only current data outside the
					// stable document; it must not contain even a fragment of it.
					dynamicAt := strings.Index(req.GetPrompt(), "<|PROMPT_SECTION_dynamic_")
					require.GreaterOrEqual(t, dynamicAt, 0)
					dynamic := req.GetPrompt()[dynamicAt:]
					require.NotContains(t, dynamic, documentSentinel)
					require.NotContains(t, dynamic, "冻结任务书：")
					require.Less(t, len(dynamic), 512, "action receipt must not grow with the plan")
					step := calls
					calls++
					switch step {
					case 8:
						return protocolResponse(c, req, native, "submit_plan", map[string]any{})
					case 17:
						return protocolResponse(c, req, native, "finish", map[string]any{})
					default:
						return protocolResponse(c, req, native, "save_evidence", map[string]any{"evidence_id": "cache.probe", "evidence_content": "same"})
					}
				}))
			require.NoError(t, err)
			defer s.Close()
			// Isolate query/status churn from legitimate time/size-triggered
			// Timeline promotion, which is tested by separate lifecycle suites.
			s.Timeline.SetTimelineBucketByteSize(-1)
			require.NoError(t, s.RunPlanOnly())
			require.Equal(t, 18, calls)
			require.Equal(t, map[string]int{"PLAN": 9, "EXEC": 9}, phaseCalls)
			minimum := 1.0
			for _, ratio := range ratios {
				if ratio < minimum {
					minimum = ratio
				}
			}
			t.Logf("native=%v: %d requests; stable prefix identical within each phase; minimum reusable message/tool bytes=%.2f%%; dynamic <=512 bytes", native, calls, minimum*100)
			if dir := os.Getenv("COORDINATOR_CONTEXT_REVIEW_DIR"); dir != "" {
				require.NoError(t, os.MkdirAll(dir, 0700))
				summary, err := json.MarshalIndent(map[string]any{"function_call": native, "requests": calls, "phase_requests": phaseCalls, "minimum_reusable_message_and_tool_byte_ratio": minimum, "stable_prefix_unchanged_within_phase": true, "tools_unchanged": true, "provider_cache_hit_rate": nil}, "", "  ")
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("plan-cache-function-call-%v.json", native)), summary, 0600))
			}
		})
	}
}
