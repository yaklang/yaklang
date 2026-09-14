package schema

import "testing"

func TestValidRuleMode(t *testing.T) {
	tests := []struct {
		in   any
		want SyntaxFlowRuleModeType
	}{
		{"source", SFR_MODE_SOURCE},
		{"SSA", SFR_MODE_SSA},
		{"pattern", SFR_MODE_SOURCE},
		{"", SFR_MODE_SSA},
		{"struct", SFR_MODE_STRUCT},
		{"STRUCT", SFR_MODE_STRUCT},
		{"unknown", SFR_MODE_SSA},
	}
	for _, tc := range tests {
		if got := ValidRuleMode(tc.in); got != tc.want {
			t.Fatalf("ValidRuleMode(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSyntaxFlowRuleNormalizeMode(t *testing.T) {
	rule := &SyntaxFlowRule{Mode: "source"}
	rule.NormalizeMode()
	if rule.Mode != SFR_MODE_SOURCE {
		t.Fatalf("explicit mode source = %q", rule.Mode)
	}

	rule = &SyntaxFlowRule{Tag: "security|source"}
	rule.NormalizeMode()
	if rule.Mode != SFR_MODE_SOURCE {
		t.Fatalf("tag infer source = %q", rule.Mode)
	}

	rule = &SyntaxFlowRule{Tag: "security"}
	rule.NormalizeMode()
	if rule.Mode != SFR_MODE_SSA {
		t.Fatalf("default ssa = %q", rule.Mode)
	}

	rule = &SyntaxFlowRule{Mode: "struct"}
	rule.NormalizeMode()
	if rule.Mode != SFR_MODE_STRUCT {
		t.Fatalf("explicit mode struct = %q", rule.Mode)
	}

	rule = &SyntaxFlowRule{Tag: "security|struct"}
	rule.NormalizeMode()
	if rule.Mode != SFR_MODE_STRUCT {
		t.Fatalf("tag infer struct = %q", rule.Mode)
	}

	rule = &SyntaxFlowRule{Mode: "unknown"}
	rule.NormalizeMode()
	if rule.Mode != SFR_MODE_SSA {
		t.Fatalf("unknown mode should default to ssa, got %q", rule.Mode)
	}
}

func TestSyntaxFlowRuleModeHelpers(t *testing.T) {
	if !(&SyntaxFlowRule{Mode: SFR_MODE_STRUCT}).IsStructMode() {
		t.Fatal("struct mode")
	}
	if !(&SyntaxFlowRule{Mode: SFR_MODE_SOURCE}).IsSourceMode() {
		t.Fatal("source mode")
	}
	if (&SyntaxFlowRule{Mode: SFR_MODE_SSA}).IsStructMode() || (&SyntaxFlowRule{Mode: SFR_MODE_SSA}).IsSourceMode() {
		t.Fatal("ssa is neither struct nor source")
	}
	if (*SyntaxFlowRule)(nil).IsStructMode() || (*SyntaxFlowRule)(nil).IsSourceMode() {
		t.Fatal("nil rule is neither")
	}
}
