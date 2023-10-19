package js2ssa

import (
	"testing"
)
func TestMain(t *testing.T) {
	prog := ParseSSA(`
	a = 0;
	if(a <= 3){
		a++;
	}`)
	prog.Show()
}
