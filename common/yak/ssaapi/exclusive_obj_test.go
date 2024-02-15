package ssaapi

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
	"testing"
)

/*
OOP MVP

# trace a.b
a = {}; a.b = 1; a.c = 3; d = a.c; // trace d -> {} & 3

# trace a.b in phi
a = {}; a.b = 1; if f { a.b = 2 }; c = a.b; // trace c -> phi{1,2} & {}

# trace dynamic member
a = {}; b = d ? "d" : e; a[b]=1; c = a[b]; // trace c -> 1
a = {}; b = d ? "d" : e; c = a[b]; // trace c -> a & "d" & e & d

# deep in closure
a = () => {return {"b": 1}}; d = a(); c = d.b; // trace c -> 1

# mask
a = {};
b = () => {a.b = 1};
c = () => {a.b = 2}
d ? b() : c();
e = a.b // mask trace e -> 1 & 2

*/

func topDefCheckMust(t *testing.T, code string, varName string, want ...any) {
	topDefCheckMustWithOpts(t, code, varName, want)
}

func topDefCheckMustWithOpts(t *testing.T, code string, varName string, want []any, opts ...OperationOption) {
	prog, err := Parse(code)
	if err != nil {
		t.Fatal(err)
	}

	om := omap.NewOrderedMap(make(map[string]bool))
	prog.Show()
	prog.Ref(varName).GetTopDefs(opts...).ForEach(func(value *Value) {
		value.Show()
		for _, w := range want {
			if strings.Contains(value.String(), fmt.Sprint(w)) {
				om.Set(fmt.Sprint(w), true)
			}
		}
	})
	if om.Len() < len(want) {
		t.Fatalf("want %d, but got %d", len(want), om.Len())
	}
	om.ForEach(func(i string, v bool) bool {
		t.Log(i)
		if !v {
			t.Errorf("want %s is not right", i)
		}
		return true
	})
}

func TestBasic_BasicObject(t *testing.T) {
	topDefCheckMust(t, `a = {}; a.b = 1; a.c = 3; d = a.c`, "d", "3")
}
func TestBasic_BasicObject2(t *testing.T) {
	topDefCheckMust(t, `a = ()=>{return {}}; a.b = 1; a.c = 3; d = a.c`, "d", "3")
}
func TestBasic_BasicObject_Trace(t *testing.T) {
	havePhi := false
	topDefCheckMustWithOpts(
		t,
		`a ={}; a.b = 1; if e {a.b=3}; d = a.b`,
		"d",
		[]any{
			"3", "1",
		},
		WithHookEveryNode(func(value *Value) error {
			if value.IsPhi() {
				havePhi = true
			}
			return nil
		}),
	)
	if !havePhi {
		t.Fatal("want to trace phi")
	}
}

func TestBasic_BasicObject_Trace2(t *testing.T) {
	havePhi := false
	topDefCheckMustWithOpts(
		t,
		`a.b=1;if c{a.b=3};d=a.b`,
		"d",
		[]any{
			"3", "1",
		},
		WithHookEveryNode(func(value *Value) error {
			if value.IsPhi() {
				havePhi = true
			}
			return nil
		}),
	)
	if !havePhi {
		t.Fatal("want to trace phi")
	}
}

func TestBasic_BasicObject_Trace3(t *testing.T) {
	havePhi := false
	topDefCheckMustWithOpts(
		t,
		`var a;a.b=1;if c{a.b=3};d=a.b`,
		"d",
		[]any{
			"3", "1",
		},
		WithHookEveryNode(func(value *Value) error {
			if value.IsPhi() {
				havePhi = true
			}
			return nil
		}),
	)
	if !havePhi {
		t.Fatal("want to trace phi")
	}
}

func TestBasic_BasicObject_Trace4(t *testing.T) {
	havePhi := false
	topDefCheckMustWithOpts(
		t,
		`a=(()=>({}))();a.b=1;if c{a.b=3};d=a.b`,
		"d",
		[]any{
			"3", "1",
		},
		WithHookEveryNode(func(value *Value) error {
			if value.IsPhi() {
				havePhi = true
			}
			return nil
		}),
	)
	if !havePhi {
		t.Fatal("want to trace phi")
	}
}

func TestBasic_Phi(t *testing.T) {
	prog, err := Parse(`a = 0; if b {a = 1;} else if e {a = 2} else {a=4}; c = a`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("d").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}

func TestOOP_Basic_Phi(t *testing.T) {
	prog, err := Parse(`a = {}; if b {aa = a; aa.b = 1;} else {a.b = 2}; c = a.b`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("d").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}

func TestOOP_Basic_DotTrace(t *testing.T) {
	prog, err := Parse(`a = {}; a.b = 1; a.c = h ? 3 : 2; d := a.c`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("d").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}

func TestObjectTest_Basic_Phi(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := Parse(`a = {"b": 1}
if f {
	a.b = 2
}
c = a.b
`)
	if err != nil {
		t.Fatal(err)
	}

	prog.Show()

	/*
		===================== Backtrack from [t-1]`1` =====================:

		->make(map[string]number).b
		  ->make(map[string]number)

		===================== Backtrack from [t-1]`1` =====================:

		->make(map[string]number).b
		  ->make(map[string]number)

		===================== Backtrack from [t0]`b` =====================:

		->make(map[string]number).b
	*/
	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			value.ShowBacktrack()
		})
	})
}

func TestObjectTest_Basic_LeftValue(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := Parse(`a = {}; if f {a.b = 2;}; e = a.b`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			value.ShowBacktrack()
		})
	})
}

func TestObjectTest_Basic_Phi2(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := Parse(`a = {"b": 1}
c = e ? "b" : j
if f {
	a[c] = 2
}
g = a[c]
h = a.c
i = a.b
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()

	/*
		===================== Backtrack from [t-1]`1` =====================:

		->make(map[string]number).phi(b111cee6-384b-4f0b-b346-daa35f00f9e3)["b",j]
		  ->make(map[string]number)
		    ->1

		===================== Backtrack from [t-1]`1` =====================:

		->make(map[string]number).phi(b111cee6-384b-4f0b-b346-daa35f00f9e3)["b",j]
		  ->make(map[string]number)
		    ->1

		===================== Backtrack from [t7]`"b"` =====================:

		->make(map[string]number).phi(b111cee6-384b-4f0b-b346-daa35f00f9e3)["b",j]
		  ->phi(b111cee6-384b-4f0b-b346-daa35f00f9e3)["b",j]
		    ->"b"

		===================== Backtrack from [t9]`j` =====================:

		->make(map[string]number).phi(b111cee6-384b-4f0b-b346-daa35f00f9e3)["b",j]
		  ->phi(b111cee6-384b-4f0b-b346-daa35f00f9e3)["b",j]
		    ->j
	*/
	prog.Ref("g").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			value.ShowBacktrack()
		})
	})
}

func TestObjectTest(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := Parse(`a = {}
a.b = 2;
if f(3) {
a.b = a.b + 1;
}

e  = () => {
	a.b += 4
}

if (f(4)) {
	e()
}
c = a.b
dump("DONE")
`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}

func TestObjectTest_2(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := Parse(`a = {}
a.b = 2;
if f(3) {
a.b = a.b + 1;
}

e  = () => {
	a.b += 4
}

if (f(4)) {
	e()
}

g = i => i.b
c = g()

dump("DONE")
`)
	if err != nil {
		t.Fatal(err)
	}

	prog.Show()
	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}

func TestObject_Basic(t *testing.T) {
	prog, err := Parse(`
	a = {}
	if c {
		a.d = 1
		println(a.d)
	}
	println(a.d)
	target = a.d
	`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("target").ShowWithSource()
}

func TestObject_Basic_Mask(t *testing.T) {
	prog, err := Parse(`
	a = {};
c = ()=>{
 dump(a.b)
}

if d {a.b=2}

c()

`)
	if err != nil {
		t.Fatalf("parse failed: %s", err)
	}
	prog.Show()
	prog.Ref("a").ShowWithSource()

}
