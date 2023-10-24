package js2ssa

import (
	"fmt"
	"testing"
)

func TestDemo1(m *testing.T) {
	prog := ParseSSA(`
	a = 2;
	`)
	prog.Show()
	fmt.Println(prog.GetErrors())
}
