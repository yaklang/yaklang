package ssa

import "testing"

func TestIsStructuralCFGBlockName(t *testing.T) {
	structural := []string{
		"", "entry", "defer",
		LoopBody, LoopHeader, "loop.latch-continue",
		IfCondition, IfDone, IfTrue, "if.false-1",
		SwitchHandler, LabelBlock,
	}
	for _, name := range structural {
		if !isStructuralCFGBlockName(name) {
			t.Fatalf("expected structural block name: %q", name)
		}
	}

	nonStructural := []string{
		"user-block", "b-3", "main",
	}
	for _, name := range nonStructural {
		if isStructuralCFGBlockName(name) {
			t.Fatalf("expected non-structural block name: %q", name)
		}
	}
}
