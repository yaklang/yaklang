package schema

import "testing"

func TestSSARiskTaxonomyPreservesRawValueAndHash(t *testing.T) {
	risk := &SSARisk{RiskType: "sqli-inject", Title: "query", RuntimeId: "run", Variable: "sink"}
	risk.Hash = risk.CalcHash()
	before := risk.Hash
	model := risk.ToGRPCModel()
	if model.RiskTypeVerbose != "SQL 注入" || model.RiskType != "sqli-inject" {
		t.Fatalf("display and raw value must remain separate: %q / %q", model.RiskTypeVerbose, model.RiskType)
	}
	if risk.CalcHash() != before || model.Hash != before || risk.RiskType != "sqli-inject" {
		t.Fatal("display taxonomy changed the raw risk value or existing hash")
	}
	for raw, want := range map[string]string{"customer-type": "customer-type", "cwe": "CWE", "custom": "自定义", "信息": "信息提示"} {
		if got := SSARiskTypeVerbose(raw); got != want {
			t.Errorf("%q: got %q, want %q", raw, got, want)
		}
	}
}
