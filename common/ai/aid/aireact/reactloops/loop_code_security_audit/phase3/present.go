package phase3

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/subagent"
	"github.com/yaklang/yaklang/common/log"
)

const forkGoalDetermineVerifyScope = "Determine code audit vulnerability verification scope"

// presentVerifyScope shows findings grouped by vulnerability type on a forked sub-agent card.
func presentVerifyScope(
	r aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
) {
	findings := state.GetFindings()
	if len(findings) == 0 {
		return
	}
	summary := formatVerifyScopeSummary(findings)
	byCategory := findingsByCategoryMap(findings)
	err := subagent.RunForkInvokerCallback(r, task, subagent.ForkJob{
		Identifier: "verify-scope",
		Goal:       forkGoalDetermineVerifyScope,
	}, func(childInvoker aicommon.AIInvokeRuntime, _ aicommon.AIStatefulTask) error {
		childInvoker.AddToTimeline("[VERIFY_SCOPE]", summary)
		if parentLoop != nil {
			emit.Phase3VerifyScope(parentLoop, findings, byCategory)
		}
		return nil
	})
	if err != nil {
		log.Warnf("[CodeAudit/Phase3] Present verify scope failed: %v", err)
		if parentLoop != nil {
			emit.Phase3VerifyScope(parentLoop, findings, byCategory)
		}
	}
}

type verifyScopeGroup struct {
	categoryID string
	findingIDs []string
}

func formatVerifyScopeSummary(findings []*model.Finding) string {
	groups := groupFindingsByCategory(findings)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("待验证 %d 个 finding，覆盖 %d 类漏洞 / %d findings across %d vulnerability types:\n",
		len(findings), len(groups), len(findings), len(groups)))
	for i, g := range groups {
		if i >= 15 {
			b.WriteString(fmt.Sprintf("  ... 另有 %d 类\n", len(groups)-15))
			break
		}
		b.WriteString(fmt.Sprintf("  • %s: %s\n", g.categoryID, strings.Join(g.findingIDs, ", ")))
	}
	return b.String()
}

func groupFindingsByCategory(findings []*model.Finding) []verifyScopeGroup {
	byCategory := findingsByCategoryMap(findings)
	order := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		order = append(order, cat)
	}
	sort.Strings(order)
	out := make([]verifyScopeGroup, 0, len(order))
	for _, cat := range order {
		out = append(out, verifyScopeGroup{categoryID: cat, findingIDs: byCategory[cat]})
	}
	return out
}

func findingsByCategoryMap(findings []*model.Finding) map[string][]string {
	byCat := make(map[string][]string)
	for _, f := range findings {
		if f == nil || f.ID == "" {
			continue
		}
		cat := strings.TrimSpace(f.Category)
		if cat == "" {
			cat = "uncategorized"
		}
		byCat[cat] = append(byCat[cat], f.ID)
	}
	for cat, ids := range byCat {
		sort.Strings(ids)
		byCat[cat] = ids
	}
	return byCat
}
