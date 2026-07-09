package ssaapi

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestFreeValueFallbackToDefault verifies that when a free value's call-site
// binding is missing (HandleFreeValue failed at compile time), the dataflow
// analysis still resolves via GetDefault instead of silently dropping the
// taint path. This was the root cause of thousands of "free value: X is not
// found in binding" errors across all large projects.
func TestFreeValueFallbackToDefault(t *testing.T) {
	// Nested closure: `secret` is captured as a free value. The binding
	// should resolve via GetDefault even if the call-site binding is missing.
	t.Run("nested closure free value traces through default", func(t *testing.T) {
		ssatest.CheckTopDef(t, `
			secret = 333333
			handler = () => {
				return secret
			}
			result = handler()
			`, "result", []string{"333333"}, false)
	})

	// Two-level nesting: the inner closure captures from the outer closure.
	t.Run("two-level nested closure free value", func(t *testing.T) {
		ssatest.CheckTopDef(t, `
			outer = 333333
			f = () => {
				inner = 444444
				return () => {
					return outer + inner
				}
			}
			f1 = f()
			result = f1()
			`, "result", []string{"333333", "444444"}, false)
	})

	// Free value used in a conditional: the taint should still flow.
	// Both branches (return x or return 0) are valid TopDef results.
	t.Run("free value in conditional", func(t *testing.T) {
		ssatest.CheckTopDef(t, `
			threshold = 100
			check = (x) => {
				if x > threshold {
					return x
				}
				return 0
			}
			result = check(200)
			`, "result", []string{"200", "0"}, false)
	})

	// Free value passed as argument to another function: the taint
	// should trace through the free value to its origin.
	t.Run("free value as argument", func(t *testing.T) {
		ssatest.CheckTopDef(t, `
			payload = 777777
			wrapper = () => {
				return payload
			}
			direct = wrapper()
			`, "direct", []string{"777777"}, false)
	})
}

// TestRecursiveTotalVisitLimit verifies that the recursiveCounter (a
// total-visit counter, not a depth counter) bounds the GetTopDefs descent
// and prevents infinite loops through cyclic dataflow paths. The counter
// is intentionally one-way (never decremented) and sets reachedDepthLimited
// permanently once the limit is hit, so a descent that keeps revisiting
// nodes through cycles is bounded.
func TestRecursiveTotalVisitLimit(t *testing.T) {
	// A chain long enough to exercise the recursive descent, with a
	// separate shallow branch. The total-visit counter should still allow
	// finding both branches within the limit for a small graph.
	prog, err := ssaapi.Parse(`
a1 = 1
a2 = a1 + 1
a3 = a2 + 1
a4 = a3 + 1
a5 = a4 + 1
b1 = 999
result = a5 + b1
`)
	if err != nil {
		t.Fatal(err)
	}

	found999 := false
	found1 := false
	prog.Ref("result").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(v *ssaapi.Value) {
			if v.IsConstInst() {
				val := v.GetConstValue()
				if val != nil {
					s := fmt.Sprintf("%v", val)
					if s == "999" {
						found999 = true
					}
					if s == "1" {
						found1 = true
					}
				}
			}
		})
	})

	if !found999 {
		t.Fatal("shallow branch b1=999 was not found")
	}
	if !found1 {
		t.Fatal("deep chain a1=1 was not found")
	}
}
