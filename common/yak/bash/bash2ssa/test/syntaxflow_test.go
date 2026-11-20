package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestBashSyntaxFlowBasic checks the def-use chain the frontend must expose for
// ordinary Bash values: assignment, positional parameters, and return status.
func TestBashSyntaxFlowBasic(t *testing.T) {
	t.Run("assignment reaches println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			sink+`
secret=token
println "$secret"
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"token"`},
			},
			bashOption,
		)
	})

	t.Run("positional parameter reaches println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			sink+`
f() {
  println "$1"
}
f payload
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"payload"`},
			},
			bashOption,
		)
	})

	t.Run("return status reaches println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			sink+`
f() {
  return 3
}
f
println "$?"
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`3`},
			},
			bashOption,
		)
	})

	t.Run("loop phi reaches println", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			sink+`
x=none
for i in a b; do
  x=$i
done
println "$x"
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"b"`, `phi`},
			},
			bashOption,
		)
	})
}

// TestBashSyntaxFlowAdvanced checks function-level queries: parameters, free
// values, cross-function dataflow, and associative array member flow.
func TestBashSyntaxFlowAdvanced(t *testing.T) {
	t.Run("function and call arguments", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			`
deploy() {
  local target=$1
  println "$target"
}
deploy payload
`,
			`deploy as $f; deploy(* as $p)`,
			map[string][]string{
				"f": {`deploy`},
				"p": {`"payload"`},
			},
			bashOption,
		)
	})

	t.Run("free value binding", func(t *testing.T) {
		code := sink + `
root=/srv
f() {
  println "$root"
}
f
`
		ssatest.CheckSyntaxFlowContain(t, code,
			`println(* as $v)`,
			map[string][]string{
				"v": {`FreeValue-root`},
			},
			bashOption,
		)
		ssatest.CheckSyntaxFlowContain(t, code,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"/srv"`},
			},
			bashOption,
		)
	})

	t.Run("cross function dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			sink+`
secret=token
send() {
  println "$1"
}
send "$secret"
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"token"`},
			},
			bashOption,
		)
	})

	t.Run("associative array member", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t,
			sink+`
key=name
declare -A rec=([name]=blue)
println "${rec[$key]}"
`,
			`println(* #-> as $v)`,
			map[string][]string{
				"v": {`"blue"`},
			},
			bashOption,
		)
	})
}
