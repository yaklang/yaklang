package ssaapi

import "testing"

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
