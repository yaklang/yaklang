package loop_scan_risk_analysis

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/schema"
)

type AnalysisPhase string

const (
	PhaseLoadMerge AnalysisPhase = "load_merge"
	PhaseFP        AnalysisPhase = "false_positive"
	PhasePOC       AnalysisPhase = "poc_generate"
	PhaseReport    AnalysisPhase = "report"
	PhaseDone      AnalysisPhase = "done"
)

type UnifiedRisk struct {
	ID              int64  `json:"id"`
	Hash            string `json:"hash"`
	RiskFeatureHash string `json:"risk_feature_hash"`
	FromRule        string `json:"from_rule"`
	RiskType        string `json:"risk_type"`
	Severity        string `json:"severity"`
	ProgramName     string `json:"program_name"`
	CodeSourceURL   string `json:"code_source_url"`
	CodeRange       string `json:"code_range"`
	CodeFragment    string `json:"code_fragment"`
	FunctionName    string `json:"function_name"`
	Line            int64  `json:"line"`
	Variable        string `json:"variable"`
	Title           string `json:"title"`
	TitleVerbose    string `json:"title_verbose"`
	Details         string `json:"details"`
	RuntimeID       string `json:"runtime_id"`
}

func newUnifiedRisk(r *schema.SSARisk) UnifiedRisk {
	if r == nil {
		return UnifiedRisk{}
	}
	return UnifiedRisk{
		ID:              int64(r.ID),
		Hash:            r.Hash,
		RiskFeatureHash: r.RiskFeatureHash,
		FromRule:        r.FromRule,
		RiskType:        r.RiskType,
		Severity:        string(r.Severity),
		ProgramName:     r.ProgramName,
		CodeSourceURL:   r.CodeSourceUrl,
		CodeRange:       r.CodeRange,
		CodeFragment:    r.CodeFragment,
		FunctionName:    r.FunctionName,
		Line:            r.Line,
		Variable:        r.Variable,
		Title:           r.Title,
		TitleVerbose:    r.TitleVerbose,
		Details:         r.Details,
		RuntimeID:       r.RuntimeId,
	}
}

// GroupMergeStats 合并后的「漏洞路径 / 代码位置」分布统计（同一组合并键下的聚合视图）。
type GroupMergeStats struct {
	RawRiskCountInGroup int `json:"raw_risk_count_in_group"`
	DistinctPaths       int `json:"distinct_paths"`
	DistinctLocations   int `json:"distinct_locations"`
	DistinctFunctions   int `json:"distinct_functions"`
	DistinctRules       int `json:"distinct_rules"`
}

type MergedRiskGroup struct {
	GroupID         string          `json:"group_id"`
	Key             string          `json:"key"`
	RiskFeatureHash string          `json:"risk_feature_hash,omitempty"`
	RiskIDs         []int64         `json:"risk_ids"`
	Paths           []string        `json:"paths"`
	Locations       []string        `json:"locations"`
	Functions       []string        `json:"functions"`
	Rules           []string        `json:"rules"`
	SeverityMax     string          `json:"severity_max"`
	Count           int             `json:"count"`
	Risks           []UnifiedRisk   `json:"risks"`
	MergeStats      GroupMergeStats `json:"merge_stats"`
}

type FPStatus string

const (
	FPNotIssue   FPStatus = "not_issue"
	FPSuspicious FPStatus = "suspicious"
	FPIsIssue    FPStatus = "is_issue"
)

type FPDecision struct {
	GroupID            string                     `json:"group_id"`
	Status             FPStatus                   `json:"status"`
	Confidence         int                        `json:"confidence"`
	IssueScore         int                        `json:"issue_score"`
	FalsePositiveScore int                        `json:"false_positive_score"`
	Reasons            []string                   `json:"reasons"`
	Evidence           []string                   `json:"evidence"`
	HistoryDisposals   []*schema.SSARiskDisposals `json:"history_disposals,omitempty"`
}

type PocArtifact struct {
	ID               string   `json:"id"`
	TargetGroupIDs   []string `json:"target_group_ids"`
	ScriptPath       string   `json:"script_path"`
	ScriptType       string   `json:"script_type"`
	Preconditions    []string `json:"preconditions"`
	Expected         string   `json:"expected"`
	SourceRiskID     int64    `json:"source_risk_id,omitempty"`
	GenerationMethod string   `json:"generation_method"`
	GenerationNote   string   `json:"generation_note,omitempty"`
}

type ReportTotals struct {
	ScanID                string `json:"scan_id"`
	OriginalRiskCount     int    `json:"original_risk_count"`
	MergedGroupCount      int    `json:"merged_group_count"`
	NotIssueCount         int    `json:"not_issue_count"`
	SuspiciousCount       int    `json:"suspicious_count"`
	IsIssueCount          int    `json:"is_issue_count"`
	NonFalsePositiveCount int    `json:"non_false_positive_count"`
	PocScriptCount        int    `json:"poc_script_count"`
}

// RiskRowSummary 逐条原始 SSA 告警的整理行：误报结论与同组合并组一致（先合并再分诊，避免对同一特征重复打分）。
type RiskRowSummary struct {
	RiskID          int64    `json:"risk_id"`
	GroupID         string   `json:"group_id"`
	MergeKey        string   `json:"merge_key"`
	Title           string   `json:"title"`
	Severity        string   `json:"severity"`
	FromRule        string   `json:"from_rule"`
	RiskType        string   `json:"risk_type"`
	CodeSourceURL   string   `json:"code_source_url"`
	Line            int64    `json:"line"`
	FunctionName    string   `json:"function_name"`
	FPStatus        FPStatus `json:"fp_status"`
	FPConfidence    int      `json:"fp_confidence"`
	FPReasonsJoined string   `json:"fp_reasons_summary"`
}

type FinalAnalysisReport struct {
	ScanID              string            `json:"scan_id"`
	TaskStatus          string            `json:"task_status"`
	Programs            []string          `json:"programs"`
	Totals              ReportTotals      `json:"totals"`
	Groups              []MergedRiskGroup `json:"groups"`
	FPDecisions         []FPDecision      `json:"fp_decisions"`
	PocArtifacts        []PocArtifact     `json:"poc_artifacts"`
	RiskRows            []RiskRowSummary  `json:"risk_rows"`
	SourceSSAReportID   int               `json:"source_ssa_report_id,omitempty"`
	SourceSSAReportPath string            `json:"source_ssa_report_path,omitempty"`
	AIFPReportMarkdown  string            `json:"ai_fp_report_markdown,omitempty"`
}

type AnalysisState struct {
	Phase                   AnalysisPhase
	ScanID                  string
	Task                    *schema.SyntaxFlowScanTask
	RawRisks                []UnifiedRisk
	Groups                  []MergedRiskGroup
	Decisions               []FPDecision
	PocArtifacts            []PocArtifact
	Report                  *FinalAnalysisReport
	WorkDir                 string
	SourceSSAReportID       int
	SourceSSAReportMarkdown string
	SourceSSAReportPath     string
	AIFPReportMarkdown      string
}

func newState(scanID string, workDir string) *AnalysisState {
	return &AnalysisState{
		Phase:   PhaseLoadMerge,
		ScanID:  strings.TrimSpace(scanID),
		WorkDir: workDir,
	}
}

func mergeKey(r UnifiedRisk) string {
	if strings.TrimSpace(r.RiskFeatureHash) != "" {
		return "feature:" + r.RiskFeatureHash
	}
	return fmt.Sprintf("fallback:%s|%s|%d|%s|%s", r.FromRule, r.CodeSourceURL, r.Line, r.FunctionName, r.Variable)
}

func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 5
	case "high":
		return 4
	case "warning", "medium":
		return 3
	case "low":
		return 2
	default:
		return 1
	}
}

func maxSeverity(a, b string) string {
	if severityRank(a) >= severityRank(b) {
		return a
	}
	return b
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// trivialFromRule detects placeholder-like rule names (heuristic only, no LLM).
func trivialFromRule(rule string) bool {
	s := strings.ToLower(strings.TrimSpace(rule))
	if s == "" {
		return true
	}
	switch s {
	case "test", "tmp", "demo", "placeholder", "foo", "bar", "abc", "sample", "example", "t", "x":
		return true
	default:
		return false
	}
}

// groupAllRisksHaveTrivialRules is true when every raw risk in the group uses a placeholder-like FromRule.
func groupAllRisksHaveTrivialRules(g MergedRiskGroup) bool {
	if len(g.Risks) == 0 {
		return false
	}
	for _, ur := range g.Risks {
		if !trivialFromRule(ur.FromRule) {
			return false
		}
	}
	return true
}

func weakRiskTitle(title string) bool {
	t := strings.TrimSpace(strings.ToLower(title))
	if t == "" {
		return true
	}
	if t == "test" || t == "demo" || t == "tmp" {
		return true
	}
	if utf8.RuneCountInString(strings.TrimSpace(title)) < 2 {
		return true
	}
	return false
}

// pocSignalForRepresentative returns PoC tier and a short note from structured fields only (no AI read of Details).
func pocSignalForRepresentative(ur UnifiedRisk) (tier string, note string) {
	if ur.ID <= 0 || strings.TrimSpace(ur.CodeSourceURL) == "" {
		return "低", "缺少 risk_id 或代码路径"
	}
	if trivialFromRule(ur.FromRule) {
		return "低", "规则名疑似测试/占位，不建议优先 ssapoc"
	}
	if weakRiskTitle(ur.Title) {
		return "需复核", "标题过短或疑似占位，请结合详情与代码再判断"
	}
	return "高", "具备 risk_id 与路径，可作 ssapoc 候选（须授权与人工复核）"
}
