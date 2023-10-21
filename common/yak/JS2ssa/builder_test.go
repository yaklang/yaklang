package js2ssa

import (
	"testing"
)

func TestDemo1(m *testing.T) {
	prog := ParseSSA(`
	a = 0;
	if(a <= 3){
		a++;
	}`)
	prog.Show()
}
