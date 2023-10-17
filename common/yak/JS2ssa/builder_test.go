package js2ssa

import (
	"testing"
)

func TestMain(t *testing.T) {
	prog := parseSSA(`
	a=0;
	a++;
	a+=1
	`)
	prog.Show()
}
