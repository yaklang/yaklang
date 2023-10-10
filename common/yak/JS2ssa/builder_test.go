package js2ssa

import (
	"testing"
)

func TestMain(t *testing.T) {
	prog := parseSSA("var a")
	prog.Show()

}