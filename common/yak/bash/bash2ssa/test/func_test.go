package test

import (
	"testing"
)

// A shell function has one variadic $@ parameter. $1, $#, $* and ${10} are
// members of that vector, so the callee body always prints a ParameterMember.
func TestBashPrintlnFunctionArguments(t *testing.T) {
	checkBashPrintln(t, `
f() {
  println "$1"
  println "$2"
  println "${10}"
  println "$#"
  println "$*"
  println "$@"
}
f "a b" "" c d e f g h i j
`,
		"ParameterMember-parameter[0].0",
		"ParameterMember-parameter[0].1",
		"ParameterMember-parameter[0].9",
		"ParameterMember-parameter[0].#",
		"ParameterMember-parameter[0].*",
		"Parameter-$@",
	)

	checkBashPrintln(t, `
f() {
  println "$1"
  println "$2"
  println "$#"
  println "$*"
}
f
`,
		"ParameterMember-parameter[0].0",
		"ParameterMember-parameter[0].1",
		"ParameterMember-parameter[0].#",
		"ParameterMember-parameter[0].*",
	)
}

func TestBashPrintlnFunctionSpreadArguments(t *testing.T) {
	checkBashPrintln(t, `
set -- a b
f() {
  println "$1"
  println "$2"
}
f "$@"
`,
		"ParameterMember-parameter[0].0",
		"ParameterMember-parameter[0].1",
	)
}

func TestBashPrintlnFunctionStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want string
	}{
		{
			name: "return value",
			code: `
f() { return 7; }
f
println "$?"
`,
			want: "7",
		},
		{
			name: "last command failure",
			code: `
f() { false; }
f
println "$?"
`,
			want: "1",
		},
		{
			name: "bare return",
			code: `
f() { return; }
f
println "$?"
`,
			want: "0",
		},
		{
			name: "return is masked to one byte",
			code: `
f() { return 300; }
f
println "$?"
`,
			want: "44",
		},
		{
			name: "negated call",
			code: `
f() { return 7; }
! f
println "$?"
`,
			want: "0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkBashPrintln(t, tc.code, tc.want)
		})
	}

	// A failed call on the left of || merges both statuses.
	checkBashPrintlnRegexp(t, `
f() { return 7; }
f || true
println "$?"
`, `^phi\(\$\?\)\[0,7\]$`)
}

func TestBashPrintlnFunctionOutput(t *testing.T) {
	checkBashPrintln(t, `
f() { echo 5; }
println "$(f)"
`, `"5"`)

	checkBashPrintln(t, `
f() { echo a; echo b; }
println "$(f)"
`, `"ab"`)

	checkBashPrintln(t, `
f() { echo 5; }
y=$(f)
println "$y"
`, `"5"`)

	// The substitution status is the function status, not the output value.
	checkBashPrintln(t, `
f() { return 3; }
y=$(f)
println "$?"
println "$y"
`, "3", `""`)
}

func TestBashPrintlnFunctionScope(t *testing.T) {
	t.Run("local shadows global", func(t *testing.T) {
		checkBashPrintln(t, `
x=global
f() {
  local x=local
  println "$x"
}
f
println "$x"
`, `"local"`, `"global"`)
	})

	t.Run("free value read", func(t *testing.T) {
		checkBashPrintln(t, `
root=/srv
f() {
  println "$root"
}
f
println "$root"
`, "FreeValue-root", `"/srv"`)
	})
}
