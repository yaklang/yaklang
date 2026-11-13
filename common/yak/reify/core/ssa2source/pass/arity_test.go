package pass_test

import (
	. "github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"testing"
)

func TestTrimInternalCallsUsesRecoveredArity(t *testing.T) {
	program := parseSSA(t, `zero
type: () -> void
entry_0:
    ret

six <i64> (1) a0, <i64> (2) a1, <i64> (3) a2, <i64> (4) a3, <i64> (5) a4, <i64> (6) a5
type: (i64, i64, i64, i64, i64, i64) -> void
entry_0:
    ret <i64> a0, <i64> a1, <i64> a2, <i64> a3, <i64> a4, <i64> a5

caller
type: () -> void
entry_0:
    <any> t1 = call <any> zero (<i64> 1, <i64> 2) binding[] member[]
    <any> t2 = call <any> six (<i64> 1, <i64> 2, <i64> 3, <i64> 4, <i64> 5, <i64> 6, <i64> 7) binding[] member[]
    ret`)
	zeroCall := findCall(t, program, "zero")
	sixCall := findCall(t, program, "six")

	TrimInternalCalls(program)
	if len(zeroCall.Args) != 0 {
		t.Fatalf("zero-parameter call args = %d", len(zeroCall.Args))
	}
	if len(sixCall.Args) != 6 {
		t.Fatalf("six-parameter call args = %d", len(sixCall.Args))
	}
}
