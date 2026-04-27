package loop_syntaxflow_scan

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

func TestDistinctFromRulesFromRisks(t *testing.T) {
	risks := []*schema.SSARisk{
		{FromRule: "r1"},
		{FromRule: "r2"},
		{FromRule: "r1"},
		nil,
	}
	got := DistinctFromRulesFromRisks(risks)
	if len(got) != 2 {
		t.Fatalf("want 2 unique, got %d: %v", len(got), got)
	}
}

func TestFormatSyntaxFlowScanEndReportMarkdownTable(t *testing.T) {
	st := &schema.SyntaxFlowScanTask{
		TaskId:        "tid-1",
		Status:        schema.SYNTAXFLOWSCAN_DONE,
		Reason:        "ok",
		Programs:      "p1",
		Kind:          "k",
		RulesCount:    3,
		TotalQuery:    10,
		SuccessQuery:  8,
		FailedQuery:   1,
		SkipQuery:     1,
		RiskCount:     5,
		CriticalCount: 0,
		HighCount:     2,
		WarningCount:  1,
		LowCount:      1,
		InfoCount:     1,
	}
	s := FormatSyntaxFlowScanEndReportMarkdownTable(st)
	if !strings.Contains(s, "tid-1") || !strings.Contains(s, "skip") {
		t.Fatalf("unexpected table: %s", s)
	}
}
