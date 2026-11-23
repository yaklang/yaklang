package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestConditionLogicOrOrder locks the symmetry of `||` inside an optional
// filter on the same chain the condition rules use: take the if condition, walk
// to the calls inside it, then filter those by name or by contained text.
//
//   $cond #> ?{opcode: call} as $calls
//   $calls?{*handle* || have: "https"} as $m
//
// The right-hand branch used to contribute nothing when it was a `have:`
// comparison, so `*x* || have:'y'` returned fewer values than `have:'y' || *x*`.
func TestConditionLogicOrOrder(t *testing.T) {
	code := `
func handle(s) { return 0 }
handle("https://example.test")
handle("strncmp")
if handle("https://example.test") == 0 { a = 1 }
if handle("strncmp") == 0 { b = 2 }
`
	// Both calls match *handle*; only the first contains "https".
	expect := map[string][]string{
		"m": {`Function-handle("https://example.test")`, `Function-handle("strncmp")`},
	}

	t.Run("glob first", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			*?{opcode: If} as $stmt
			$stmt<cfgConditionValues> as $cond
			$cond #> ?{opcode: call} as $calls
			$calls?{*handle* || have: "https"} as $m
			print $m
		`, expect)
	})

	t.Run("have first", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
			*?{opcode: If} as $stmt
			$stmt<cfgConditionValues> as $cond
			$cond #> ?{opcode: call} as $calls
			$calls?{have: "https" || *handle*} as $m
			print $m
		`, expect)
	})
}