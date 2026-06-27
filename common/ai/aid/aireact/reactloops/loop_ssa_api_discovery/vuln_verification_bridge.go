package loop_ssa_api_discovery

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

const (
	vulnVerificationSourceSyntaxflow = "syntaxflow"
	vulnVerificationSourceDynamic    = "dynamic"
)

// BridgeDynamicFindingToVulnVerification upserts a vuln_verifications row for a dynamic greybox finding.
func BridgeDynamicFindingToVulnVerification(rt *Runtime, finding *store.DynamicVulnFinding) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil || finding == nil {
		return nil
	}
	if finding.Status != "confirmed" {
		return nil
	}
	existing, err := rt.Repo.GetVulnVerificationByDynamicFindingID(rt.Session.ID, finding.ID)
	if err == nil && existing != nil {
		return nil
	}
	conf := finding.Confidence / 10
	if conf < 1 {
		conf = 7
	}
	if conf > 10 {
		conf = 10
	}
	row := &store.VulnVerification{
		SessionID:          rt.Session.ID,
		DynamicFindingID:   finding.ID,
		Source:             vulnVerificationSourceDynamic,
		Status:             "confirmed",
		Confidence:         conf,
		ExploitPayload:     finding.Payload,
		ExploitResponse:    utilsShrinkDynamicResponse(finding),
		AIAnalysis:         finding.AIAnalysis,
		Fix:                fmt.Sprintf("[%s] %s severity=%s endpoint_id=%d", finding.VulnType, finding.Evidence, finding.Severity, finding.HttpEndpointID),
	}
	return rt.Repo.UpsertVulnVerificationByDynamicFinding(row)
}

// BridgeAllConfirmedDynamicFindings syncs all confirmed dynamic findings into vuln_verifications.
func BridgeAllConfirmedDynamicFindings(rt *Runtime) (int, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, nil
	}
	rows, err := rt.Repo.ListDynamicVulnFindingsByStatus(rt.Session.ID, "confirmed")
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range rows {
		if err := BridgeDynamicFindingToVulnVerification(rt, &rows[i]); err != nil {
			log.Warnf("ssa_api_discovery: bridge dynamic finding id=%d: %v", rows[i].ID, err)
			continue
		}
		n++
	}
	return n, nil
}

func utilsShrinkDynamicResponse(f *store.DynamicVulnFinding) string {
	if f == nil {
		return ""
	}
	parts := []string{}
	if u := strings.TrimSpace(f.RequestURL); u != "" {
		parts = append(parts, "url="+u)
	}
	if e := strings.TrimSpace(f.Evidence); e != "" {
		parts = append(parts, e)
	}
	if r := strings.TrimSpace(f.ResponseRaw); r != "" {
		if len(r) > 4000 {
			r = r[:4000] + "..."
		}
		parts = append(parts, r)
	}
	return strings.Join(parts, "\n")
}
