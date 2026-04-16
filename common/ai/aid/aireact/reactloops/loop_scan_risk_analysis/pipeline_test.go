package loop_scan_risk_analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

func TestParseScanID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "scan_id=task-123", want: "task-123"},
		{in: "task_id: abcdefgh", want: "abcdefgh"},
		{in: "12345678-aaaa", want: "12345678-aaaa"},
	}
	for _, c := range cases {
		got := parseScanID(c.in)
		if got != c.want {
			t.Fatalf("parseScanID(%q)=%q want=%q", c.in, got, c.want)
		}
	}
}

func TestMergeKeyFallbackAndFeature(t *testing.T) {
	a := UnifiedRisk{RiskFeatureHash: "rfh-1"}
	if got := mergeKey(a); got != "feature:rfh-1" {
		t.Fatalf("unexpected feature merge key: %s", got)
	}

	b := UnifiedRisk{
		FromRule:      "ruleA",
		CodeSourceURL: "a/b.go",
		Line:          10,
		FunctionName:  "f",
		Variable:      "v",
	}
	if !strings.HasPrefix(mergeKey(b), "fallback:ruleA|a/b.go|10|f|v") {
		t.Fatalf("unexpected fallback merge key: %s", mergeKey(b))
	}
}

func TestGeneratePOCScriptsAndReports(t *testing.T) {
	workDir := t.TempDir()
	s := newState("scan-task-001", workDir)
	s.Groups = []MergedRiskGroup{
		{
			GroupID:     "G-0001",
			Key:         "feature:test-1",
			SeverityMax: "high",
			Rules:       []string{"sql-inject-rule"},
			Functions:   []string{"query"},
			Risks: []UnifiedRisk{
				{ID: 101, Title: "SQLi", Severity: "high", FromRule: "sql-inject-rule", CodeSourceURL: "pkg/a.go", Line: 12, FunctionName: "query", CodeFragment: "SELECT *"},
			},
			MergeStats: GroupMergeStats{RawRiskCountInGroup: 1, DistinctPaths: 1, DistinctLocations: 1, DistinctFunctions: 1, DistinctRules: 1},
		},
		{
			GroupID:     "G-0002",
			Key:         "feature:test-2",
			SeverityMax: "low",
			Rules:       []string{"xss-rule"},
			Functions:   []string{"render"},
			Risks: []UnifiedRisk{
				{ID: 202, Title: "XSS", Severity: "low", FromRule: "xss-rule", CodeSourceURL: "pkg/b.go", Line: 3, FunctionName: "render"},
			},
			MergeStats: GroupMergeStats{RawRiskCountInGroup: 1, DistinctPaths: 1, DistinctLocations: 1, DistinctFunctions: 1, DistinctRules: 1},
		},
	}
	s.Decisions = []FPDecision{
		{GroupID: "G-0001", Status: FPIsIssue, Confidence: 8},
		{GroupID: "G-0002", Status: FPNotIssue, Confidence: 7},
	}

	s.Phase = PhaseReport
	if len(s.PocArtifacts) != 0 {
		t.Fatalf("want 0 poc artifacts before report (no auto PoC) got %d", len(s.PocArtifacts))
	}

	if err := s.generateReports(); err != nil {
		t.Fatalf("generateReports err: %v", err)
	}
	if s.Report == nil {
		t.Fatalf("want non-nil report")
	}
	if s.Report.Totals.PocScriptCount != 0 {
		t.Fatalf("want PocScriptCount 0 got %d", s.Report.Totals.PocScriptCount)
	}
	if len(s.Report.RiskRows) != 2 {
		n := 0
		if s.Report != nil {
			n = len(s.Report.RiskRows)
		}
		t.Fatalf("want 2 risk_rows got len=%d", n)
	}

	baseDir := filepath.Join(workDir, "scan_risk_analysis", "scan-task-001")
	for _, f := range []string{
		"analysis_summary.json",
		"analysis_report.md",
		"poc_manifest.json",
		"false_positive_report.md",
		"poc_generation_report.md",
	} {
		if _, err := os.Stat(filepath.Join(baseDir, f)); err != nil {
			t.Fatalf("expected report file %s not found: %v", f, err)
		}
	}
}

func TestTrivialFromRuleAndPocSignal(t *testing.T) {
	if !trivialFromRule("test") || !trivialFromRule("  TEST ") {
		t.Fatalf("trivialFromRule should match test")
	}
	if trivialFromRule("检测Go语言xorm SQL注入漏洞") {
		t.Fatalf("real rule name should not be trivial")
	}
	low := UnifiedRisk{ID: 10, CodeSourceURL: "x.go", Line: 1, FromRule: "test", Title: "anything"}
	if tier, _ := pocSignalForRepresentative(low); tier != "低" {
		t.Fatalf("placeholder rule want tier 低 got %q", tier)
	}
	review := UnifiedRisk{ID: 10, CodeSourceURL: "x.go", Line: 1, FromRule: "real-rule", Title: "test"}
	if tier, _ := pocSignalForRepresentative(review); tier != "需复核" {
		t.Fatalf("weak title want 需复核 got %q", tier)
	}
	g := MergedRiskGroup{
		Risks: []UnifiedRisk{
			{ID: 1, FromRule: "test", CodeSourceURL: "a.go", Title: "t1"},
			{ID: 2, FromRule: "real-sql", CodeSourceURL: "b.go", Title: "t2"},
		},
	}
	picked := pickRepresentativeRiskForPoc(g)
	if picked.FromRule != "real-sql" {
		t.Fatalf("pickRepresentativeRiskForPoc want non-trivial rule, got %q", picked.FromRule)
	}
}

func TestScoreSampledRiskContent_RuleNameIsWeakSignal(t *testing.T) {
	strong := []UnifiedRisk{
		{
			ID:            11,
			FromRule:      "test",
			RiskType:      "SQL注入",
			Title:         "SQL Injection Risk",
			Details:       "user input flows into SQL query, sink is raw select",
			CodeFragment:  `db.Raw("select * from users where id=" + id)`,
			CodeSourceURL: "controllers/user.go",
			Line:          42,
		},
	}
	signals := scoreSampledRiskContent(strong)
	if signals.IssueDelta <= signals.FalseDelta {
		t.Fatalf("expect strong content to prefer issue side, got issue=%d false=%d", signals.IssueDelta, signals.FalseDelta)
	}

	weak := []UnifiedRisk{
		{
			ID:            12,
			FromRule:      "real-rule",
			RiskType:      "SQL注入",
			Title:         "demo",
			Details:       "demo placeholder test only",
			CodeFragment:  "",
			CodeSourceURL: "controllers/demo.go",
			Line:          1,
		},
	}
	weakSignals := scoreSampledRiskContent(weak)
	if weakSignals.FalseDelta <= weakSignals.IssueDelta {
		t.Fatalf("expect weak content to prefer false-positive side, got issue=%d false=%d", weakSignals.IssueDelta, weakSignals.FalseDelta)
	}
}

func TestExtractMarkdownFromReportItems(t *testing.T) {
	r := &schema.Report{}
	r.Markdown("alpha")
	r.Divider()
	r.Markdown("  beta  ")
	got := extractMarkdownFromReportItems(r)
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
		t.Fatalf("unexpected markdown join: %q", got)
	}
}

func TestParseFPVerdictJSONArray(t *testing.T) {
	if _, err := parseFPVerdictJSONArray("not json"); err == nil {
		t.Fatalf("expected error for invalid json")
	}
	raw, _ := json.Marshal(map[string]any{
		"verdicts": []map[string]any{
			{"group_id": "G-0001", "status": "not_issue", "confidence": 7, "reason": "x", "evidence": []string{"e1"}},
		},
	})
	vs, err := parseFPVerdictJSONArray(string(raw))
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if len(vs) != 1 || vs[0].GroupID != "G-0001" {
		t.Fatalf("unexpected verdicts: %+v", vs)
	}
}

func TestFalsePositiveReportInnerShowsContentDrivenSuspicious(t *testing.T) {
	report := &FinalAnalysisReport{
		ScanID: "scan-1",
		FPDecisions: []FPDecision{
			{
				GroupID:    "G-0001",
				Status:     FPSuspicious,
				Confidence: 6,
				Reasons:    []string{"风险内容偏弱，需要人工复核"},
				Evidence:   []string{"content_fp_signal_keywords=1"},
			},
		},
		RiskRows: []RiskRowSummary{
			{
				RiskID:        1001,
				GroupID:       "G-0001",
				Title:         "demo risk",
				Severity:      "low",
				FromRule:      "rule-a",
				CodeSourceURL: "controllers/demo.go",
				Line:          9,
				FPStatus:      FPSuspicious,
			},
		},
	}

	md := falsePositiveReportInner(report)
	if !strings.Contains(md, "疑似误报 · 合并组（suspicious") {
		t.Fatalf("expected suspicious section in markdown, got: %s", md)
	}
	if !strings.Contains(md, "| 1001 | G-0001 |") {
		t.Fatalf("expected suspicious risk row in markdown, got: %s", md)
	}
}
