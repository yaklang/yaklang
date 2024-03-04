package ssaapi

import (
	"fmt"
	"testing"
)

func ParseMustForTestCase(code string) *Program {
	prog, err := Parse(code)
	if err != nil {
		panic(err)
	}
	return prog
}

func TestSideEffect_Case1(t *testing.T) {
	prog := ParseMustForTestCase(`
a = 1
b = () => {
	a = 2
}
b()
c = a;
`)
	cRef := prog.Ref("c")
	cRefToFunc := false
	cRefTo2 := false
	cRef.Show()
	results := cRef.GetTopDefs().Show().ForEach(func(value *Value) {
		if value.IsCall() {
			cRefToFunc = true
		}
		if fmt.Sprint(value.GetConstValue()) == `2` {
			cRefTo2 = true
		}
	})
	if len(results) != 2 {
		t.Fatalf("Expect 2 results, but got %d", len(results))
	}
	if !cRefToFunc {
		t.Fatal("Expect c ref defsto to c, but not")
	}

	if !cRefTo2 {
		t.Fatal("Expect c ref defsto to 2, but not")
	}
}

func TestSideEffect_Obj(t *testing.T) {
	prog := ParseMustForTestCase(`
a = {}
b = () => {
	a.b = 2
}
b()
c = a.b;
`)
	cRef := prog.Ref("c")
	cRefToFunc := false
	cRefTo2 := false
	results := cRef.GetTopDefs().Show().ForEach(func(value *Value) {
		if value.IsCall() {
			cRefToFunc = true
		}
		if fmt.Sprint(value.GetConstValue()) == `2` {
			cRefTo2 = true
		}
	})
	if len(results) != 2 {
		t.Fatalf("Expect 2 results, but got %d", len(results))
	}
	if !cRefToFunc {
		t.Fatal("Expect c ref defsto to function, but not")
	}

	if !cRefTo2 {
		t.Fatal("Expect c ref defsto to 2, but not")
	}
}

func TestSideEffect_Case2(t *testing.T) {
	prog := ParseMustForTestCase(`
a = 1
b = () => {
	a = 2
}
if e {b()}
c = a;
`)
	cRef := prog.Ref("c")
	cRefToFunc := false
	cRefTo2 := false
	cRefTo1 := false
	results := cRef.GetTopDefs().Show().ForEach(func(value *Value) {
		if value.IsCall() {
			cRefToFunc = true
		}
		if fmt.Sprint(value.GetConstValue()) == `2` {
			cRefTo2 = true
		}
		if fmt.Sprint(value.GetConstValue()) == `1` {
			cRefTo1 = true
		}
	})
	if len(results) < 3 {
		t.Fatalf("Expect >=3 results, but got %d", len(results))
	}
	if !cRefToFunc {
		t.Fatal("Expect c ref defsto to function, but not")
	}

	if !cRefTo2 {
		t.Fatal("Expect c ref defsto to 2, but not")
	}
	if !cRefTo1 {
		t.Fatal("Expect c ref defsto to 1, but not")
	}

	if !cRef.Get(0).IsPhi() {
		t.Fatal("c is phi must")
	}
}

func TestSideEffect_Obj2(t *testing.T) {
	prog := ParseMustForTestCase(`
a = 1
b = () => {
	a = 2
}
if e {b()}
c = a;
`)
	cRef := prog.Ref("c")
	cRefToFunc := false
	cRefTo2 := false
	cRefTo1 := false
	results := cRef.GetTopDefs().Show().ForEach(func(value *Value) {
		if value.IsCall() {
			cRefToFunc = true
		}
		if fmt.Sprint(value.GetConstValue()) == `2` {
			cRefTo2 = true
		}
		if fmt.Sprint(value.GetConstValue()) == `1` {
			cRefTo1 = true
		}
	})
	if len(results) < 3 {
		t.Fatalf("Expect >=3 results, but got %d", len(results))
	}
	if !cRefToFunc {
		t.Fatal("Expect c ref defsto to function, but not")
	}

	if !cRefTo2 {
		t.Fatal("Expect c ref defsto to 2, but not")
	}
	if !cRefTo1 {
		t.Fatal("Expect c ref defsto to 1, but not")
	}

	if !cRef.Get(0).IsPhi() {
		t.Fatal("c is phi must")
	}
}

func TestSideEffect_BottomUse(t *testing.T) {
	prog := ParseMustForTestCase(`
a = 5
b = () => {
	a = 2
}
if e {b()}
c = a+1;
`)
	aRef := prog.Ref("a").Filter(func(value *Value) bool {
		return value.GetConstValue() == 5
	})
	aRef.Show()
	checkPhi := false
	checkVal5 := false
	aRef.GetBottomUses().Show().ForEach(func(value *Value) {
		value.GetOperands().ForEach(func(value *Value) {
			if value.IsPhi() {
				checkPhi = true
				value.GetOperands().ForEach(func(value *Value) {
					if value.GetConstValue() == 5 {
						checkVal5 = true
					}
				})
			}
		})
	})
	if !checkVal5 {
		t.Fatal("expect 5, but not")
	}
	if !checkPhi {
		t.Fatal("expect phi, but not")
	}
}
