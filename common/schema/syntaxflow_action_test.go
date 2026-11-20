package schema

import "testing"

func TestSyntaxFlowActionForSeverity(t *testing.T) {
	cases := []struct {
		severity SyntaxFlowSeverity
		want     SyntaxFlowAction
	}{
		{SFR_SEVERITY_CRITICAL, SFR_ACTION_DENY},
		{SFR_SEVERITY_HIGH, SFR_ACTION_DENY},
		{SFR_SEVERITY_WARNING, SFR_ACTION_ASK},
		{"mid", SFR_ACTION_ASK},
		{"warn", SFR_ACTION_ASK},
		{SFR_SEVERITY_LOW, SFR_ACTION_WARN},
		{SFR_SEVERITY_INFO, SFR_ACTION_ALLOW},
		{"", SFR_ACTION_ALLOW},
		{"unknown", SFR_ACTION_ALLOW},
	}
	for _, tc := range cases {
		if got := SyntaxFlowActionForSeverity(tc.severity); got != tc.want {
			t.Fatalf("severity %q: want action %q, got %q", tc.severity, tc.want, got)
		}
	}
}

func TestSyntaxFlowActionRank(t *testing.T) {
	ordered := []SyntaxFlowAction{
		SFR_ACTION_ALLOW,
		SFR_ACTION_WARN,
		SFR_ACTION_ASK,
		SFR_ACTION_DENY,
	}
	for i := 1; i < len(ordered); i++ {
		if ordered[i].Rank() <= ordered[i-1].Rank() {
			t.Fatalf("action %q must rank above %q", ordered[i], ordered[i-1])
		}
	}
	if !SFR_ACTION_DENY.IsBlocking() {
		t.Fatal("deny must be blocking")
	}
	for _, action := range []SyntaxFlowAction{SFR_ACTION_ALLOW, SFR_ACTION_WARN, SFR_ACTION_ASK} {
		if action.IsBlocking() {
			t.Fatalf("action %q must not be blocking", action)
		}
	}
}
