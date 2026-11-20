package test

import (
	"testing"
)

func TestBashPrintlnClosure(t *testing.T) {
	t.Run("free value", func(t *testing.T) {
		checkBashPrintlnContain(t, `
x=outer
capture() {
  println "$x"
}
capture
`, "FreeValue-x")
	})

	t.Run("local shadows global", func(t *testing.T) {
		checkBashPrintln(t, `
x=outer
f() {
  local x=inner
  println "$x"
}
f
println "$x"
`, `"inner"`, `"outer"`)
	})

	t.Run("subshell scope", func(t *testing.T) {
		checkBashPrintln(t, `
x=outer
out=$(x=inner; println "$x")
println "$x"
`, `"inner"`, `"outer"`)
	})

	t.Run("brace group and subshell", func(t *testing.T) {
		checkBashPrintlnRegexp(t, `
x=parent
out=$(x=child; get "$x")
println "$x"
println "$out"
{ x=group; }
( x=sub )
println "$x"
`, `^"parent"$`, `get.*"child"`, `^"group"$`)
	})

	t.Run("temporary assignment and pipeline", func(t *testing.T) {
		checkBashPrintln(t, `
x=parent
show "$x" | { x=pipeline; }
x=temporary show arg
println "$x"
`, `"parent"`)
	})
}
