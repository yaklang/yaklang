package ssaapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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

func topDefCheckMustWithOpts(t *testing.T, code string, varName string, want []any, opts ...ssaapi.OperationOption) {
	prog, err := ssaapi.Parse(code)
	if err != nil {
		t.Fatal(err)
	}

	om := omap.NewOrderedMap(make(map[string]bool))
	prog.Show()
	prog.Ref(varName).GetTopDefs(opts...).ForEach(func(value *ssaapi.Value) {
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
	ssatest.CheckTopDef(t, `
       a = {}; 
       a.b = 1; 
       a.c = 3; 
       d = a.c + a.b
       `, "d", []string{"3", "1"}, true)
}

func TestBasic_BasicObject2(t *testing.T) {
	ssatest.CheckTopDef(t, `
       a = ()=>{return {}}; 
       a.b = 1; 
       a.c = 3; 
       d = a.c + a.b
       `, "d", []string{"3", "1"}, true)
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
		ssaapi.WithHookEveryNode(func(value *ssaapi.Value) error {
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
		ssaapi.WithHookEveryNode(func(value *ssaapi.Value) error {
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
		ssaapi.WithHookEveryNode(func(value *ssaapi.Value) error {
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
		ssaapi.WithHookEveryNode(func(value *ssaapi.Value) error {
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
	prog, err := ssaapi.Parse(`a = 0; if b {a = 1;} else if e {a = 2} else {a=4}; c = a`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("d").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			t.Log(value.String())
		})
	})
}

func TestOOP_Basic_Phi(t *testing.T) {
	prog, err := ssaapi.Parse(`a = {}; if b {aa = a; aa.b = 1;} else {a.b = 2}; c = a.b`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("d").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			t.Log(value.String())
		})
	})
}

func TestOOP_Basic_DotTrace(t *testing.T) {
	prog, err := ssaapi.Parse(`a = {}; a.b = 1; a.c = h ? 3 : 2; d := a.c`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("d").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			t.Log(value.String())
		})
	})
}

func TestObjectTest_Basic_Phi(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := ssaapi.Parse(`a = {"b": 1}
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
	prog.Ref("c").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			value.ShowBacktrack()
		})
	})
}

func TestObjectTest_Basic_LeftValue(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := ssaapi.Parse(`a = {}; if f {a.b = 2;}; e = a.b`)
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	prog.Ref("c").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			value.ShowBacktrack()
		})
	})
}

func TestObjectTest_Basic_Phi2(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := ssaapi.Parse(`a = {"b": 1}
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
	prog.Ref("g").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			value.ShowBacktrack()
		})
	})
}

func TestObjectTest(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := ssaapi.Parse(`a = {}
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
	prog.Ref("c").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			t.Log(value.String())
		})
	})
}

func TestObjectTest_2(t *testing.T) {
	// a.b can be as "phi and masked"
	prog, err := ssaapi.Parse(`a = {}
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
c = g(a)

dump("DONE")
`)
	if err != nil {
		t.Fatal(err)
	}

	prog.Show()
	prog.Ref("c").ForEach(func(value *ssaapi.Value) {
		value.GetTopDefs().ForEach(func(value *ssaapi.Value) {
			t.Log(value.String())
		})
	})
}

func TestObject_Basic(t *testing.T) {
	prog, err := ssaapi.Parse(`
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
	prog, err := ssaapi.Parse(`
	a = {};
c = ()=>{
 dump(a.b)
}

if d {a.b=2}

c()

`)
	if err != nil {
		t.Fatalf("ssaapi.Parse failed: %s", err)
	}
	prog.Show()
	prog.Ref("a").ShowWithSource()

}

func TestObject_OOP_ClassAndObject(t *testing.T) {
	prog, err := ssaapi.Parse(`
klass = (name) => {
	this = {
		"name": name,
		"getName": () => this.name,
		"setName": i => {this.name = i}
	}
	return this
}

obj1 := klass("abc")
obj1.setName("def")
c = obj1.name
dump(c)


`)
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	prog.Ref("c").GetTopDefs().Show().ForEach(func(value *ssaapi.Value) {
		if value.GetConstValue() == "def" {
			checked = true
		}
	})
	if !checked {
		t.Fatal("oop trace failed")
	}

}

func TestObject_OOP_ClassAndObject_Dup2(t *testing.T) {
	prog, err := ssaapi.Parse(`
klass = (name) => {
	this = {
		"name": name,
		"getName": () => this.name,
		"setName": i => {this.name = i}
	}
	return this
}

obj1 := klass("abc")
obj1.setName("def")
c = obj1.name

obj2 := klass("cccc")
obj2.setName("dddd")
d = obj2.name

obj2 := klass("cccc")
e = obj2.name

`)
	if err != nil {
		t.Fatal(err)
	}
	check_C := false
	check_D := false
	check_E := false
	prog.Ref("c").GetTopDefs().Show().ForEach(func(value *ssaapi.Value) {
		if value.GetConstValue() == "def" {
			check_C = true
		}
	})
	prog.Ref("d").GetTopDefs().Show().ForEach(func(value *ssaapi.Value) {
		if value.GetConstValue() == "dddd" {
			check_D = true
		}
	})
	prog.Ref("e").GetTopDefs().ForEach(func(value *ssaapi.Value) {
		if value.GetConstValue() == "cccc" {
			check_E = true
		}
	})

	if !check_C {
		t.Fatal("oop trace failed for C")
	}

	if !check_D {
		t.Fatal("oop trace failed for D")
	}

	if !check_E {
		t.Fatal("oop trace failed for E")
	}
}

func TestObject_OOP_class(t *testing.T) {
	prog, err := ssaapi.Parse(`
a = () => {
	return {
		"key": "value"
	}
}

c = a().key
dump(c)
`)
	if err != nil {
		t.Fatal(err)
	}
	checkDotKey := false
	prog.Show().Ref("c").GetTopDefs().ForEach(func(value *ssaapi.Value) {
		if value.GetConstValue() == "value" {
			checkDotKey = true
		}
	})
	if !checkDotKey {
		t.Fatal("oop trace failed")
	}
}

func TestObject_OOP_class_2(t *testing.T) {
	prog, err := ssaapi.Parse(`
klass = () => {
	this = {
		"key": "value",
		"changeKey": i => {this.key = i}
	}
	return this
}

obj := klass()
obj.changeKey("kkk")
c = obj.key
dump(c)
`)
	if err != nil {
		t.Fatal(err)
	}
	checkDotKey := false
	checkMaskedKey := false
	prog.Show().Ref("c").GetTopDefs().ForEach(func(value *ssaapi.Value) {
		// if value.GetConstValue() == "value" { // will cover by side-effect
		// 	checkDotKey = true
		// }
		if value.GetConstValue() == "kkk" {
			checkMaskedKey = true
		}
	}).Show()
	_ = checkDotKey
	// if !checkDotKey {
	// 	t.Fatal("oop trace failed")
	// }
	_ = checkMaskedKey
	if !checkMaskedKey {
		t.Fatal("oop trace masked failed")
	}
}

func TestObject_Make(t *testing.T) {
	t.Run("make bottom user", func(t *testing.T) {
		code := `
		m = []string{"i","j"}
		print(m[1])
		`
		ssatest.CheckSyntaxFlowContain(t, code,
			`
			e"j" as $j
			$j --> as $target
			`, map[string][]string{
				"target": {`Undefined-print("j")`},
			}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
