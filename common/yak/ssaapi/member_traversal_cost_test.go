package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// This file pins the fan-out of a bottom-use descent over an object whose type
// exposes both data fields and methods, in the shape that made a heavy rule
// exhaust the dataflow value limit on a real project.
//
// Java registers a class's methods in the same member table as its fields. A
// method member is not data: the call site that reads `obj.m()` already carries
// `obj` as its receiver, so the flow is carried by the object's own uses.
// Walking a method member as if it were a field makes the descent follow every
// call site of that method -- on a class blueprint, every method of the class,
// each pulling in its own caller set.
//
// The assertions below are deliberately two-sided:
//
//	counted - bottom-use must only enumerate the fields it actually walks, not
//	          the class's method surface;
//	flow    - the value written into the field must still reach the sink, so a
//	          change that lowers the count by dropping the real path fails too.

// costClassJava declares a class with two data fields and six methods. The
// method count is what makes the difference measurable: before the traversal
// fix, each method was walked as data.
const costClassJava = `
package com.example;

class Box {
    public String cmd;
    public String decoy;

    void run() { Sink.consume(this.cmd); }

    void p1() {}
    void p2() {}
    void p3() {}
    void p4() {}
}

class Sink {
    static void consume(Object o) {}
}

class CaseCost {
    public void assign(String cmd) {
        Box box = new Box();
        box.cmd = cmd;
        box.decoy = "d";
        box.run();
    }
}
`

// TestMemberTraversalSkipsCallableSurface asserts that tracing the class
// blueprint enumerates only its data fields. The blueprint exposes 6 members
// (2 fields + 4 methods); walking the method surface pulls in each method's
// callers and inflates both the enumerated count and the result.
func TestMemberTraversalSkipsCallableSurface(t *testing.T) {
	prev := setDataflowTraceEnabled(true)
	t.Cleanup(func() { setDataflowTraceEnabled(prev) })

	prog, err := Parse(costClassJava, WithLanguage(ssaconfig.JAVA))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	res, err := prog.SyntaxFlowWithError(`Box as $v`)
	if err != nil {
		t.Fatalf("syntaxflow failed: %v", err)
	}

	var checked int
	for _, v := range res.GetValues("v") {
		if !v.IsObject() {
			continue
		}
		all := v.GetAllMember()
		if len(all) < 6 {
			// Not the class blueprint (instances carry only their data fields).
			continue
		}
		checked++

		lastWidenTrace.Store(nil)
		out := v.GetBottomUses()
		tr := lastWidenTrace.Load()
		if tr == nil {
			t.Fatalf("tracer did not record the descent")
		}

		enumerated := tr.membersEnumerated.Load()
		maxPerObject := tr.maxMembersPerObject.Load()
		t.Logf("members=%d out=%d membersEnumerated=%d maxPerObject=%d",
			len(all), len(out), enumerated, maxPerObject)

		// The blueprint has exactly 2 data fields. Enumerating more than a
		// small multiple means the method surface is being walked as data.
		if maxPerObject > 2 {
			t.Errorf("one expansion enumerated %d members; the method surface is being walked as data (fields: 2)", maxPerObject)
		}
		if enumerated > 4 {
			t.Errorf("bottom-use enumerated %d members; the class only has 2 data fields", enumerated)
		}
	}
	if checked == 0 {
		t.Fatalf("no class blueprint matched; the test did not exercise the traversal")
	}
}

// TestMemberTraversalReachesFieldThroughCallPath keeps the other side honest:
// the value assigned to the field must still travel the real call path into the
// method body, and the unread sibling field must not appear.
func TestMemberTraversalReachesFieldThroughCallPath(t *testing.T) {
	prog, err := Parse(costClassJava, WithLanguage(ssaconfig.JAVA))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	res, err := prog.SyntaxFlowWithError(`consume(* #-> * as $target)`)
	if err != nil {
		t.Fatalf("syntaxflow failed: %v", err)
	}
	got := map[string]bool{}
	for _, v := range res.GetValues("target") {
		got[v.String()] = true
	}
	if len(got) == 0 {
		t.Fatalf("sink matched nothing; the forward flow is missing")
	}
	if got[`"d"`] {
		t.Errorf("unread sibling field reached the sink: %v", got)
	}
}
