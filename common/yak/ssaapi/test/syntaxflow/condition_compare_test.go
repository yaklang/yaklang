package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestConditionConstCompare keeps the shape of the "condition compares against
// a constant" rule covered inside the test suite instead of a committed .sf:
// take the if condition, keep the binary comparisons, then reach their constant
// operand.
func TestConditionConstCompare(t *testing.T) {
	code := `
func compare(a, b, c) {
	if a == 1 { x = 10 }
	if b != 2 { x = 11 }
	if !(c == 3) { x = 12 }
}
`
	ssatest.CheckSyntaxFlow(t, code, `
		*?{opcode: If || opcode: Loop} as $stmt
		$stmt<cfgConditionValues> as $cond

		$cond?{opcode: eq || opcode: neq || opcode: gt || opcode: gteq || opcode: lt || opcode: lteq} as $binDirect
		$cond #> ?{opcode: eq || opcode: neq || opcode: gt || opcode: gteq || opcode: lt || opcode: lteq} as $binNested
		$binDirect + $binNested as $bin

		$bin #> ?{opcode: const} as $const
		print $bin
	`, map[string][]string{
		"bin": {
			`eq(Parameter-a, 1)`,
			`neq(Parameter-b, 2)`,
			`eq(Parameter-c, 3)`,
		},
	})
}

// TestConditionCallCompare keeps the shape of the "comparison is a call with a
// literal argument" rule covered. In C the string comparison lives in a call
// (`strcmp(x, "https") == 0`), so the rule walks from the condition to the
// calls, filters them by name, and reads the constant arguments.
func TestConditionCallCompare(t *testing.T) {
	code := `
func strcmp_ish(a) { return 0 }
func strnicmp_ish(a) { return 0 }
func unrelated(a) { return 0 }

if strcmp_ish("https") == 0 { x = 1 }
if strnicmp_ish("http") == 0 { x = 2 }
if unrelated("https") == 0 { x = 3 }
`
	ssatest.CheckSyntaxFlow(t, code, `
		*?{opcode: If || opcode: Loop} as $stmt
		$stmt<cfgConditionValues> as $cond

		$cond #> ?{opcode: call} as $calls
		$calls?{*strcmp* || *strnicmp*} as $call

		$call #> ?{opcode: const} as $arg
		print $call
	`, map[string][]string{
		"call": {
			`Function-strcmp_ish("https")`,
			`Function-strnicmp_ish("http")`,
		},
	})
}

// TestConditionCallNameGlobWithDash covers the name pattern that made the glob
// token necessary: SSA prints a call as `Function-<name>(...)`, so matching a
// callee by text needs a '-' inside the pattern.
func TestConditionCallNameGlobWithDash(t *testing.T) {
	code := `
func check(a) { return 0 }
if check("x") == 0 { y = 1 }
`
	ssatest.CheckSyntaxFlow(t, code, `
		*?{opcode: If} as $stmt
		$stmt<cfgConditionValues> as $cond
		$cond #> ?{opcode: call} as $calls
		$calls?{*Function-check*} as $call
		print $call
	`, map[string][]string{
		"call": {`Function-check("x")`},
	})
}
// TestConditionIndexedRead covers the "indexed read inside the condition"
// filter that the sloppy-check triage used: `s[4] == 'x'` reaches the SSA as
// `eq(ParameterMember-parameter[0].4, ...)`, so `?{opcode: param_member}`
// finds the read. (That triage rule is not shipped; the mechanism it needed is
// locked here.)
func TestConditionIndexedRead(t *testing.T) {
	code := `
func check(s) {
	if s[4] == "s" { x = 1 }
}
`
	ssatest.CheckSyntaxFlow(t, code, `
		*?{opcode: If} as $stmt
		$stmt<cfgConditionValues> as $cond
		$cond #> ?{opcode: param_member} as $indexed
		print $indexed
	`, map[string][]string{
		"indexed": {`ParameterMember-parameter[0].4`},
	})
}
