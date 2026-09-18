package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// The bottom-use descent keeps a stack of entered *ssa.Call frames so that a
// Parameter / ParameterMember callee can be bound back to the function it
// actually receives at that call site (peekCall(1) = "the call that invoked the
// current function"). That lookup is only meaningful if the stack mirrors the
// live nest of entered calls.
//
// The bug this guards against: pushCall had no matching pop, so every call ever
// entered stayed on the stack. A second call in the same function body then saw
// its earlier *sibling* as its parent and resolved its parameter to that
// sibling's argument.
//
// The assertions below drive a real descent and check the stack invariants
// directly, plus the observable consequence: two sibling calls that receive
// different arguments must not cross-bind.

// TestCallStackBalancesAcrossSiblingCalls pins that the call stack returns to
// its starting depth after a descent that enters nested calls.
func TestCallStackBalancesAcrossSiblingCalls(t *testing.T) {
	const src = `package main

func sink(any) {}

func inner(a string) {
	sink(a)
}

func outer(first string, second string) {
	inner(first)
	inner(second)
}

func run(p string, q string) {
	outer(p, q)
}
`
	prog, err := Parse(src, WithLanguage(ssaconfig.GO))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	res, err := prog.SyntaxFlowWithError(`sink(* as $v)`)
	if err != nil {
		t.Fatalf("syntaxflow failed: %v", err)
	}
	vals := res.GetValues("v")
	if len(vals) == 0 {
		t.Fatalf("sink matched nothing")
	}

	for _, v := range vals {
		actx := NewAnalyzeContext()
		actx.Self = v
		actx.direct = BottomUseAnalysis

		base := actx.callStack.Len()
		v.getBottomUses(actx)
		if got := actx.callStack.Len(); got != base {
			t.Errorf("call stack leaked after descent: started at %d, ended at %d", base, got)
		}
	}
}

// TestCallStackSiblingCallsKeepOwnArguments records the behaviour that a paired
// call stack protects: two sibling method calls on one receiver each resolve
// their own argument.
//
// NOTE: this case currently passes even with the pop removed -- the shape does
// not reach the upward stack walk. It is kept as a behaviour record, not as the
// guard for the pairing; TestCallStackBalancesAcrossSiblingCalls is the guard.
func TestCallStackDoesNotCrossBindSiblingArguments(t *testing.T) {
	const src = `<?php
class A{
    public function b($x){ println($x); }
}
function bBB($a){
    $a->b(1);
    $a->b(2);
}
$a = new A();
bBB($a);
`
	prog, err := Parse(src, WithLanguage(ssaconfig.PHP))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	res, err := prog.SyntaxFlowWithError(`A() --> as $v`)
	if err != nil {
		t.Fatalf("syntaxflow failed: %v", err)
	}
	got := map[string]bool{}
	for _, v := range res.GetValues("v") {
		got[v.String()] = true
	}
	if len(got) == 0 {
		t.Fatalf("rule matched nothing")
	}
	if !got["ParameterMember-parameter[0].b(Parameter-$a,1)"] {
		t.Errorf("first sibling call lost its own argument: %v", got)
	}
	if !got["ParameterMember-parameter[0].b(Parameter-$a,2)"] {
		t.Errorf("second sibling call lost its own argument: %v", got)
	}
}
