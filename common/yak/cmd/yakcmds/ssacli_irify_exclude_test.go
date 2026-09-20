//go:build irify_exclude

package yakcmds

import "testing"

func TestSlimGate_NoSSACompilerCommands(t *testing.T) {
	if len(SSACompilerCommands) != 0 {
		t.Fatal("slim must not advertise unavailable SSA compiler/scan commands")
	}
}
