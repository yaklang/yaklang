package loop_scan_risk_analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	_ "github.com/yaklang/yaklang/common/yak" // register ExecuteForge for ExecuteForgeFromDB / ssapoc
)

const maxSsapocTargetsPerScan = 8
const perRiskSsapocTimeout = 25 * time.Minute

type ssapocPick struct {
	Group MergedRiskGroup
	Risk  UnifiedRisk
	FP    FPDecision
}

// collectSsapocTargets picks at most one representative risk per group after FP triage:
// only is_issue / suspicious, must have risk_id and code path for ssapoc.
func collectSsapocTargets(s *AnalysisState) []ssapocPick {
	decBy := make(map[string]FPDecision, len(s.Decisions))
	for _, d := range s.Decisions {
		decBy[d.GroupID] = d
	}
	var out []ssapocPick
	for _, g := range s.Groups {
		d, ok := decBy[g.GroupID]
		if !ok || d.Status == FPNotIssue {
			continue
		}
		var rep UnifiedRisk
		for _, ur := range g.Risks {
			if ur.ID <= 0 {
				continue
			}
			if strings.TrimSpace(ur.CodeSourceURL) == "" {
				continue
			}
			rep = ur
			break
		}
		if rep.ID == 0 {
			continue
		}
		out = append(out, ssapocPick{Group: g, Risk: rep, FP: d})
		if len(out) >= maxSsapocTargetsPerScan {
			break
		}
	}
	return out
}

func buildSsapocAdditionalContext(s *AnalysisState, p ssapocPick) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"scan_risk_analysis | scan/runtime_id=%s | group_id=%s | merge_key=%s | fp=%s conf=%d\n",
		s.ScanID, p.Group.GroupID, p.Group.Key, p.FP.Status, p.FP.Confidence,
	))
	if len(p.FP.Reasons) > 0 {
		b.WriteString("fp_reasons: " + strings.Join(p.FP.Reasons, " | ") + "\n")
	}
	b.WriteString(fmt.Sprintf("from_rule=%s risk_type=%s title=%s\n",
		p.Risk.FromRule, p.Risk.RiskType, strings.TrimSpace(strings.ReplaceAll(p.Risk.Title, "\n", " "))))
	if det := strings.TrimSpace(p.Risk.Details); det != "" {
		if len(det) > 2000 {
			det = det[:2000] + "...(truncated)"
		}
		b.WriteString("risk_details:\n" + det)
	}
	return b.String()
}

func forgeOptionsForSsapoc(ctx context.Context, inv aicommon.AIInvokeRuntime) []aicommon.ConfigOption {
	opts := []aicommon.ConfigOption{aicommon.WithContext(ctx)}
	if inv == nil {
		return opts
	}
	cfg := inv.GetConfig()
	if c, ok := cfg.(*aicommon.Config); ok && c != nil {
		opts = append(opts, aicommon.ConvertConfigToOptions(c)...)
	}
	return opts
}

func trySsapocForge(ctx context.Context, inv aicommon.AIInvokeRuntime, s *AnalysisState, p ssapocPick, savePath string) (method, note string) {
	if inv == nil {
		return "rule_template_fallback", "ssapoc skipped: invoker nil"
	}
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		return "rule_template_fallback", fmt.Sprintf("mkdir save_path: %v", err)
	}
	params := []*ypb.ExecParamItem{
		{Key: "risk_id", Value: fmt.Sprintf("%d", p.Risk.ID)},
		{Key: "additional_context", Value: buildSsapocAdditionalContext(s, p)},
		{Key: "save_path", Value: savePath},
		{Key: "target_base_url", Value: "http://target.example.com"},
		{Key: "strict_mode", Value: "true"},
	}
	runCtx, cancel := context.WithTimeout(ctx, perRiskSsapocTimeout)
	defer cancel()
	opts := forgeOptionsForSsapoc(runCtx, inv)
	if _, err := aicommon.ExecuteForgeFromDB("ssapoc", runCtx, params, opts...); err != nil {
		log.Warnf("[ScanRisk] ssapoc risk_id=%d: %v", p.Risk.ID, err)
		return "rule_template_fallback", "ssapoc: " + err.Error()
	}
	return "ssapoc", "ssapoc finished; outputs under save_path"
}

func writeTemplateFallbackArtifact(s *AnalysisState, g MergedRiskGroup, root string, sourceRiskID int64) (PocArtifact, error) {
	scriptType := "http"
	ext := ".http"
	category := "generic"
	riskType := strings.ToLower(strings.Join(g.Rules, " ") + " " + strings.Join(g.Functions, " "))
	switch {
	case strings.Contains(riskType, "sql"):
		scriptType, ext, category = "python", ".py", "sql_injection"
	case strings.Contains(riskType, "command"), strings.Contains(riskType, "exec"):
		scriptType, ext, category = "shell", ".sh", "command_execution"
	case strings.Contains(riskType, "xss"), strings.Contains(riskType, "ssrf"):
		scriptType, ext, category = "http", ".http", "web_injection"
	}
	path := filepath.Join(root, strings.ToLower(g.GroupID)+ext)
	content := buildPOCScript(g, category, scriptType)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return PocArtifact{}, err
	}
	sid := sourceRiskID
	if sid == 0 && len(g.Risks) > 0 {
		sid = g.Risks[0].ID
	}
	return PocArtifact{
		ID:               g.GroupID,
		TargetGroupIDs:   []string{g.GroupID},
		ScriptPath:       path,
		ScriptType:       scriptType,
		Preconditions:    []string{"Authorized testing only", "Replace URLs/parameters for your target"},
		Expected:         "Observe vulnerable behavior in controlled environment",
		SourceRiskID:     sid,
		GenerationMethod: "rule_template_fallback",
		GenerationNote:   "Placeholder when ssapoc is unavailable or failed",
	}, nil
}

// runScanRiskPocPhase runs after FP triage: eligible risks -> builtin ssapoc forge, else template file.
func runScanRiskPocPhase(ctx context.Context, inv aicommon.AIInvokeRuntime, s *AnalysisState) error {
	root := filepath.Join(s.WorkDir, "scan_risk_analysis", s.ScanID, "artifacts", "poc")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var artifacts []PocArtifact
	for _, pick := range collectSsapocTargets(s) {
		savePath := filepath.Join(root, fmt.Sprintf("risk_%d_ssapoc", pick.Risk.ID))
		method, note := trySsapocForge(ctx, inv, s, pick, savePath)
		if method == "ssapoc" {
			artifacts = append(artifacts, PocArtifact{
				ID:               pick.Group.GroupID,
				TargetGroupIDs:   []string{pick.Group.GroupID},
				ScriptPath:       savePath,
				ScriptType:       "ssapoc_workspace",
				Preconditions:    []string{"Review files under save_path"},
				Expected:         "Python PoC or artifacts from ssapoc",
				SourceRiskID:     pick.Risk.ID,
				GenerationMethod: "ssapoc",
				GenerationNote:   note,
			})
			continue
		}
		a, err := writeTemplateFallbackArtifact(s, pick.Group, root, pick.Risk.ID)
		if err != nil {
			return err
		}
		a.GenerationNote = strings.TrimSpace(note + " | " + a.GenerationNote)
		artifacts = append(artifacts, a)
	}
	s.PocArtifacts = artifacts
	s.Phase = PhaseReport
	return nil
}
