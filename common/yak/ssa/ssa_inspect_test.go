package ssa

import "testing"

func TestSSA_SMOKING(t *testing.T) {
	prog := ParseSSA(`var a = 1;c= a + 1; a = c + 2;`)
	prog.Show()
}
