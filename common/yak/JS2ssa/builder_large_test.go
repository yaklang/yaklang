package js2ssa

import (
	_ "embed"
	"testing"
)

//go:embed test.js
var largeJS string

func checkLarge(t *testing.T, code string) {
	prog := ParseSSA(code, none)
	prog.ShowWithSource()
}

func TestLargeText(t *testing.T) {
	ParseSSA(largeJS, nil).Show()
}
