package ssaapi

import (
	"testing"
)

// TestParse_BrokenIfCondition_NoPanic ensures that incomplete/broken if
// conditions produced by error-recovery do not cause a nil pointer panic in
// the SSA builder (regression for If.SetCondition on a nil condition value).
func TestParse_BrokenIfCondition_NoPanic(t *testing.T) {
	codes := []string{
		"if result, err = tls.",
		"if a[0] = ",
		"if a. {}",
		"if {}",
	}
	for _, code := range codes {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("unexpected panic on code %q: %v", code, r)
				}
			}()
			// error is acceptable (syntax error), panic is not.
			_, _ = Parse(code)
		}()
	}
}
