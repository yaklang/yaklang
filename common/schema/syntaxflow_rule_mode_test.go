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
}
