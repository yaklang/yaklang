package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

func validateEndpointDeepMiningCoverage(sessionID, apiID uint, probes []store.EndpointVulnProbe) error {
	required := AllVulnTypeIDs()
	if len(required) == 0 {
		return utils.Error("empty vuln type registry")
	}
	byType := map[string]store.EndpointVulnProbe{}
	for _, p := range probes {
		byType[strings.TrimSpace(p.VulnType)] = p
	}
	var missing []string
	for _, id := range required {
		p, ok := byType[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		if p.Status == "skipped" && strings.TrimSpace(p.SkipReason) == "" {
			return utils.Errorf("endpoint %d vuln_type=%s skipped without skip_reason", apiID, id)
		}
		if strings.TrimSpace(p.Status) == "" {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return utils.Errorf("endpoint %d missing vuln probe records for: %s", apiID, strings.Join(missing, ", "))
	}
	_ = sessionID
	return nil
}

func syncConfirmedProbeToDynamicFinding(rt *Runtime, target HttpProbeTarget, probe store.EndpointVulnProbe) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	if probe.Status != "confirmed" {
		return nil
	}
	def, ok := VulnTypeDefByID(probe.VulnType)
	severity := "high"
	if ok && def.Name != "" {
		_ = def
	}
	epID := target.HttpEndpointID
	if epID == 0 {
		ids := HttpEndpointIDsFromProbeTargets(rt, []HttpProbeTarget{target})
		if len(ids) > 0 {
			epID = ids[0]
		}
	}
	row := &store.DynamicVulnFinding{
		SessionID:      rt.Session.ID,
		HttpEndpointID: epID,
		VulnType:       probe.VulnType,
		Severity:       severity,
		Confidence:     80,
		Payload:        probe.Payload,
		RequestURL:     probe.RequestURL,
		ResponseRaw:    probe.ResponseExcerpt,
		Evidence:       probe.AIAnalysis,
		Status:         "confirmed",
		AIAnalysis:     probe.AIAnalysis,
		CodeContext:    target.CodeSnippet,
	}
	if err := rt.Repo.CreateDynamicVulnFinding(row); err != nil {
		return err
	}
	return BridgeDynamicFindingToVulnVerification(rt, row)
}

func writeDeepMiningSkippedReport(path string, reason string) error {
	body := fmt.Sprintf(`# [阶段 4/5 - Step3] Phase4 Step3: 深度挖掘漏洞检测

## 执行摘要

**未执行**深度挖掘：%s

## 说明

- 深度挖掘模式要求 verified=true 且 full_sample_url 非空的 probe target
- 请完善 Phase1 HTTP 验证后再续跑
`, reason)
	return os.WriteFile(path, []byte(body), 0o644)
}
