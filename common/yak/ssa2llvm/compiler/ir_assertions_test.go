package compiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireNoLLVMPhiNodes(t *testing.T, ir string) {
	t.Helper()
	for _, line := range strings.Split(ir, "\n") {
		if strings.Contains(line, " phi ") {
			t.Fatalf("unexpected LLVM phi node: %q", line)
		}
	}
}

func requireIRContainsGlobalCString(t *testing.T, ir, literal string) {
	t.Helper()
	needle := "c\"" + literal + "\\00\""
	require.Contains(t, ir, needle, "expected global string literal %q in IR", literal)
}

func requireIRContainsSlotLowering(t *testing.T, ir string) {
	t.Helper()
	require.Contains(t, ir, "yak_slot_")
	require.Contains(t, ir, "yak_load_")
}
