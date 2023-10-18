package js2ssa

import (
	"testing"
)

func TestMain(t *testing.T) {
	prog := parseSSA(`
	for(a = 0;a<3;a++){}
	`)
	prog.Show()
}
