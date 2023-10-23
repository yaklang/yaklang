package js2ssa

import (
	"testing"
)

func TestDemo1(m *testing.T) {
	prog := ParseSSA(`
	b = 1
	function functionName(arg1, arg2) {
		a = arg1 + arg2;
		return a;
	}
	b = functionName(1, 2);
	print(b);
	`)
	prog.Show()
}
