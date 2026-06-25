package loop_ssa_api_discovery

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// ResolvePipelineStartStage 返回应从哪一平台阶段（1～5）开始执行；6 表示已全部完成。
func ResolvePipelineStartStage(parsed *ParsedUserInput, sess *store.DiscoverySession) int {
	if parsed != nil && parsed.PipelineResumeFromStage > 0 {
		return clampPipelineStartStage(parsed.PipelineResumeFromStage)
	}
	if parsed != nil && parsed.PipelineResume && sess != nil {
		return ResumeStageFromSessionPhase(sess.Phase)
	}
	return 1
}

// ResumeStageFromSessionPhase 根据 session.phase 推断续跑起始阶段（1～5）；6=已完成。
func ResumeStageFromSessionPhase(phase string) int {
	p := strings.TrimSpace(strings.ToLower(phase))
	switch p {
	case PhasePipelineReport, "completed", "done":
		return 6
	case PhaseVulnVerified:
		return 5
	case PhaseVulnScanned:
		return 4
	case PhaseApiVerified:
		return 2
	case PhaseStep0ChecklistDone, PhaseStep1AuthDone, PhaseStep2StaticDone:
		return 4
	case PhaseInitialized, "":
		return 1
	default:
		if strings.HasPrefix(p, "phase5_step") {
			return 4
		}
		return 1
	}
}

func clampPipelineStartStage(n int) int {
	if n < PipelineStageMin {
		return PipelineStageMin
	}
	if n > PipelineStageFullMax {
		return PipelineStageFullMax
	}
	return n
}
