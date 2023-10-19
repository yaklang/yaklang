package js2ssa

import (
	"testing"
)
func TestMain(t *testing.T) {
	prog := parseSSA(`
	function myFunction(){
		a = 0;
	}`)
	prog.Show()
}
