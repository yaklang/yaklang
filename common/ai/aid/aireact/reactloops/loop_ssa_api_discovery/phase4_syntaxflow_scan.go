package loop_ssa_api_discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SyntaxFlowScanMeta Phase4 扫描审计元数据（写入 discovery_sessions.syntax_flow_scan_meta_json 与 syntaxflow_summary.json）。
type SyntaxFlowScanMeta struct {
	Executed      bool           `json:"executed"`
	Source        string         `json:"source"`
	FilterSummary map[string]any `json:"filter_summary,omitempty"`
	RuleNames     []string       `json:"rule_names"`
	RulesQueued   int            `json:"rules_queued"`
	RisksImported int            `json:"risks_imported"`
	ScanError     string         `json:"scan_error,omitempty"`
}

// ParseSyntaxFlowScanMeta 从会话字段反序列化元数据（可能为空）。
func ParseSyntaxFlowScanMeta(raw string) (*SyntaxFlowScanMeta, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var m SyntaxFlowScanMeta
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func filterToSummary(f *ypb.SyntaxFlowRuleFilter) map[string]any {
	if f == nil {
		return map[string]any{"mode": "all_rules_in_profile_db"}
	}
	return map[string]any{
		"language":          f.GetLanguage(),
		"severity":          f.GetSeverity(),
		"tag":               f.GetTag(),
		"keyword":           f.GetKeyword(),
		"rule_names":        f.GetRuleNames(),
		"group_names":       f.GetGroupNames(),
		"filter_rule_kind":  f.GetFilterRuleKind(),
		"filter_lib_kind":   f.GetFilterLibRuleKind(),
		"purpose":           f.GetPurpose(),
	}
}

func queryRuleNamesForFilter(filter *ypb.SyntaxFlowRuleFilter) ([]string, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Error("profile database unavailable")
	}
	q := yakit.FilterSyntaxFlowRule(db, filter)
	var names []string
	err := q.Order("rule_name asc").Pluck("rule_name", &names).Error
	if err != nil {
		return nil, err
	}
	return names, nil
}

func persistSyntaxFlowScanMeta(rt *Runtime, meta *SyntaxFlowScanMeta) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil || meta == nil {
		return utils.Error("nil runtime or meta")
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	rt.Session.SyntaxFlowScanMetaJSON = string(b)
	if err := rt.Repo.UpdateSession(rt.Session); err != nil {
		return err
	}
	return nil
}

func importSyntaxFlowRisksToSession(rt *Runtime, risks []*schema.SSARisk) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	sess := rt.Session
	if derr := rt.Repo.DeleteDiscoverySyntaxFlowFindingsBySession(sess.ID); derr != nil {
		log.Warnf("ssa_api_discovery: delete old sf findings: %v", derr)
	}
	for _, risk := range risks {
		line := int(risk.Line)
		sev := string(risk.Severity)
		row := &store.DiscoverySyntaxFlowFinding{
			SessionID:    sess.ID,
			RiskHash:     risk.Hash,
			RuleName:     risk.FromRule,
			Severity:     sev,
			Title:        risk.Title,
			Description:  risk.Details,
			MatchedFile:  risk.CodeSourceUrl,
			MatchedLine:  line,
			DataFlowHint: risk.CodeFragment,
			Confidence:   severityToConfidence(sev),
		}
		if row.Title == "" {
			row.Title = risk.RiskType
		}
		if err := rt.Repo.CreateDiscoverySyntaxFlowFinding(row); err != nil {
			log.Warnf("ssa_api_discovery: insert sf finding: %v", err)
		}
	}
	return nil
}

// RunSyntaxFlowScan 按给定 Profile 规则过滤器执行 SyntaxFlow 扫描，写入 SSARisk → discovery_syntaxflow_findings、会话元数据与 syntaxflow_summary.json。
func RunSyntaxFlowScan(ctx context.Context, rt *Runtime, pl *PipelineState, filter *ypb.SyntaxFlowRuleFilter, source string) (err error) {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return fmt.Errorf("nil runtime")
	}
	step := fmt.Sprintf("phase3.syntaxflow_scan.%s", source)
	execType := "programmatic"
	if source == "ai" {
		execType = "ai+programmatic"
	}
	started := time.Now()
	outFiles := []string{store.SyntaxflowSummaryPath(rt.WorkDir)}
	rt.execStepStart(step, execType)
	defer func() {
		if err != nil {
			rt.execStepError(step, execType, started, err, outFiles)
		} else {
			rt.execStepEnd(step, execType, started, outFiles)
		}
	}()
	sess := rt.Session
	meta := &SyntaxFlowScanMeta{
		Source:        source,
		FilterSummary: filterToSummary(filter),
	}

	if !sess.SSACompileOK || strings.TrimSpace(sess.SSAProgramName) == "" {
		log.Infof("ssa_api_discovery: skip SyntaxFlow scan (ssa_ok=false or empty program)")
		_ = rt.Repo.AppendEvent(sess.ID, "info", "syntaxflow_skip", `{"reason":"no_ssa_program"}`)
		meta.Executed = false
		meta.ScanError = "no_ssa_program"
		_ = persistSyntaxFlowScanMeta(rt, meta)
		return writeSyntaxFlowSummaryFile(rt, pl, nil, meta)
	}

	names, err := queryRuleNamesForFilter(filter)
	if err != nil {
		log.Warnf("ssa_api_discovery: query rules for filter: %v", err)
		meta.ScanError = err.Error()
		meta.Executed = false
		_ = persistSyntaxFlowScanMeta(rt, meta)
		_ = writeSyntaxFlowSummaryFile(rt, pl, nil, meta)
		return err
	}
	meta.RuleNames = names
	meta.RulesQueued = len(names)

	if len(names) == 0 {
		log.Infof("ssa_api_discovery: SyntaxFlow zero rules matched filter (source=%s)", source)
		meta.Executed = true
		meta.RisksImported = 0
		_ = persistSyntaxFlowScanMeta(rt, meta)
		_ = rt.Repo.AppendEvent(sess.ID, "info", "syntaxflow_scan", `{"rules":0,"source":"`+source+`"}`)
		return writeSyntaxFlowSummaryFile(rt, pl, nil, meta)
	}

	opts := []ssaconfig.Option{
		ssaconfig.WithProgramNames(sess.SSAProgramName),
		ssaconfig.WithScanConcurrency(3),
		// syntaxflow_scan.Scan 要求 ControlMode 为 start|status|resume，否则返回 invalid syntaxFlow scan mode
		ssaconfig.WithScanControlMode(ssaconfig.ControlModeStart),
	}
	if filter != nil {
		opts = append(opts, ssaconfig.WithRuleFilter(filter))
	}
	scanCtx, cancelScan := detachSyntaxFlowScanContext(ctx)
	defer cancelScan()
	if err := syntaxflow_scan.Scan(scanCtx, opts...); err != nil {
		log.Warnf("ssa_api_discovery: SyntaxFlow scan: %v", err)
		meta.ScanError = err.Error()
		_ = rt.Repo.AppendEvent(sess.ID, "warn", "syntaxflow_scan", fmt.Sprintf("%q", err.Error()))
	}

	dbSSA := consts.GetGormSSAProjectDataBase()
	var risks []*schema.SSARisk
	if dbSSA != nil {
		var qerr error
		_, risks, qerr = yakit.QuerySSARisk(dbSSA, &ypb.SSARisksFilter{
			ProgramName: []string{sess.SSAProgramName},
		}, &ypb.Paging{Page: 1, Limit: 2000})
		if qerr != nil {
			log.Warnf("ssa_api_discovery: QuerySSARisk: %v", qerr)
		}
	}
	if risks == nil {
		risks = []*schema.SSARisk{}
	}
	if err := importSyntaxFlowRisksToSession(rt, risks); err != nil {
		log.Warnf("ssa_api_discovery: import risks: %v", err)
	}
	meta.Executed = true
	meta.RisksImported = len(risks)
	_ = persistSyntaxFlowScanMeta(rt, meta)
	_ = rt.Repo.AppendEvent(sess.ID, "info", "syntaxflow_scan", fmt.Sprintf(`{"risks_imported":%d,"rules_queued":%d,"source":%q}`, len(risks), meta.RulesQueued, source))
	return writeSyntaxFlowSummaryFile(rt, pl, risks, meta)
}

// FallbackSyntaxFlowRuleFilter 退回策略：内置规则 + 会话语言（若有）。
func FallbackSyntaxFlowRuleFilter(sess *store.DiscoverySession) *ypb.SyntaxFlowRuleFilter {
	f := &ypb.SyntaxFlowRuleFilter{
		FilterRuleKind: yakit.FilterBuiltinRuleTrue,
	}
	lang := strings.TrimSpace(sess.Language)
	if lang != "" {
		f.Language = []string{strings.ToLower(lang)}
	}
	return f
}

func syntaxflowScanNeedsFallback(rt *Runtime) bool {
	if rt == nil || rt.Session == nil {
		return true
	}
	if !rt.Session.SSACompileOK || strings.TrimSpace(rt.Session.SSAProgramName) == "" {
		return false
	}
	meta, err := ParseSyntaxFlowScanMeta(rt.Session.SyntaxFlowScanMetaJSON)
	if err != nil || meta == nil {
		return true
	}
	if !meta.Executed {
		return true
	}
	if meta.RulesQueued == 0 {
		return true
	}
	if meta.RisksImported == 0 && meta.RulesQueued > 0 {
		return true
	}
	return false
}

func severityToConfidence(sev string) int {
	switch schema.ValidSeverityType(sev) {
	case schema.SFR_SEVERITY_CRITICAL:
		return 9
	case schema.SFR_SEVERITY_HIGH:
		return 7
	case schema.SFR_SEVERITY_WARNING:
		return 5
	case schema.SFR_SEVERITY_LOW:
		return 4
	default:
		return 3
	}
}

func writeSyntaxFlowSummaryFile(rt *Runtime, pl *PipelineState, risks []*schema.SSARisk, meta *SyntaxFlowScanMeta) error {
	if rt == nil || rt.Session == nil {
		return fmt.Errorf("nil runtime")
	}
	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	path := filepath.Join(dir, "syntaxflow_summary.json")
	_ = os.MkdirAll(dir, 0o755)
	n := 0
	if risks != nil {
		n = len(risks)
	}
	payload := map[string]any{
		"program_name": rt.Session.SSAProgramName,
		"risk_count":   n,
		"risks":        risks,
	}
	if meta != nil {
		payload["scan_meta"] = meta
		payload["rule_names"] = meta.RuleNames
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	pl.SetSyntaxFlowJSONPath(path)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	if rt.Repo != nil {
		return rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactSyntaxflowSummary, string(b))
	}
	return nil
}
