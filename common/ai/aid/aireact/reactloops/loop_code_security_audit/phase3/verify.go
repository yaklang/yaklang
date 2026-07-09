package phase3

import (
	_ "embed"
	"fmt"
	"math"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed prompts/verify_instruction.txt
var phase3VerifyInstruction string

//go:embed prompts/output_example.txt
var phase3OutputExample string

// BuildVerifyLoop builds the Phase 3 orchestrator loop.
// It forks one sub-agent per finding (concurrency=5) and merges verified_vulns.json at the end.
func BuildVerifyLoop(r aicommon.AIInvokeRuntime, state *model.AuditState, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(false),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			findings := state.GetFindings()
			log.Infof("[CodeAudit/Phase3] Verify orchestrator started, %d findings", len(findings))

			if len(findings) == 0 {
				r.AddToTimeline("[VERIFY_INIT]", "没有 finding 需要验证，跳过验证阶段。")
				op.Done()
				return
			}

			state.DedupeVerifiedVulns()
			emit.Phase3VerifyStart(loop, len(findings))
			presentVerifyScope(r, loop, task, state)

			outcomes := runAllFindingVerifications(r, loop, task, state)

			state.DedupeVerifiedVulns()
			state.SetPhase(model.AuditPhaseReport)

			stats := state.GetStats()
			verified := state.GetVerifiedVulns()
			confirmed := state.GetConfirmedVulns()
			incomplete := 0
			for _, o := range outcomes {
				if o.incomplete {
					incomplete++
				}
			}

			r.AddToTimeline("[VERIFY_COMPLETE]", fmt.Sprintf(
				"Phase 3 验证完成。共 %d 个 finding，确认: %d，uncertain: %d，safe: %d（未完成子任务: %d）",
				len(findings), len(confirmed), stats.UncertainCount, stats.SafeCount, incomplete))

			log.Infof("[CodeAudit/Phase3] Verify orchestrator complete. total=%d verified=%d confirmed=%d uncertain=%d safe=%d incomplete=%d",
				len(findings), len(verified), len(confirmed), stats.UncertainCount, stats.SafeCount, incomplete)
			op.Done()
		}),
	}

	preset = append(preset, opts...)
	return reactloops.NewReActLoop("code_audit_phase3_orchestrator", r, preset...)
}
