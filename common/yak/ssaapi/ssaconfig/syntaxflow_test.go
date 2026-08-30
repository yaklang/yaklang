package ssaconfig

import "testing"

func TestDefaultScanRuleWorkLimit(t *testing.T) {
	if DefaultScanRuleWorkLimit != 50_000 {
		t.Fatalf("DefaultScanRuleWorkLimit = %d, want 50000", DefaultScanRuleWorkLimit)
	}
}
