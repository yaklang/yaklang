package phase2

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/subagent"
	"github.com/yaklang/yaklang/common/log"
)

const forkGoalDetermineAuditVulnerabilityTypes = "Determine code audit vulnerability types"

// presentAuditVulnerabilityTypes shows the finalized audit vulnerability types on a forked sub-agent card.
func presentAuditVulnerabilityTypes(
	r aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	categories []model.VulnCategory,
) {
	if len(categories) == 0 {
		return
	}
	summary := formatAuditVulnerabilityTypesSummary(categories)
	err := subagent.RunForkInvokerCallback(r, task, subagent.ForkJob{
		Identifier: "audit-vuln-types",
		Goal:       forkGoalDetermineAuditVulnerabilityTypes,
	}, func(childInvoker aicommon.AIInvokeRuntime, _ aicommon.AIStatefulTask) error {
		childInvoker.AddToTimeline("[AUDIT_VULN_TYPES]", summary)
		if parentLoop != nil {
			emit.Phase2AuditVulnerabilityTypes(parentLoop, categories)
		}
		return nil
	})
	if err != nil {
		log.Warnf("[CodeAudit/Phase2] Present audit vulnerability types failed: %v", err)
		if parentLoop != nil {
			emit.Phase2AuditVulnerabilityTypes(parentLoop, categories)
		}
	}
}

func formatAuditVulnerabilityTypesSummary(categories []model.VulnCategory) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("本次审计将覆盖 %d 类漏洞类型 / Audit covers %d vulnerability types:\n", len(categories), len(categories)))
	for i, c := range categories {
		if i >= 20 {
			b.WriteString(fmt.Sprintf("  ... 另有 %d 类\n", len(categories)-20))
			break
		}
		b.WriteString(fmt.Sprintf("  %d. %s (%s)\n", i+1, c.Name, c.ID))
	}
	return b.String()
}
