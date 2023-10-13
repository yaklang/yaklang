package js2ssa

import (
	"testing"
)

func TestMain(t *testing.T) {
	prog := parseSSA(`var a = 1 + 2;`)
	prog.Show()
}
