package ssaapi

import "testing"

func TestObjectTest_Basic_Phi(t *testing.T) {
	// a.b can be as "phi and masked"
	prog := Parse(`a = {"b": 1}
if f {
	a.b = 2
}
c = a.b
`).Show()

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

func TestObjectTest_Basic_Phi_1(t *testing.T) {
	// a.b can be as "phi and masked"
	prog := Parse(`a = {}; if f {a.b = 2;}; e = a.b
`).Show()
	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			value.ShowBacktrack()
		})
	})
}

func TestObjectTest_Basic_Phi2(t *testing.T) {
	// a.b can be as "phi and masked"
	prog := Parse(`a = {"b": 1}
c = e ? "b" : j
if f {
	a[c] = 2
}
g = a[c]
h = a.c
i = a.b
`).Show()

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
	prog := Parse(`a = {}
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
`).Show()

	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}

func TestObjectTest_2(t *testing.T) {
	// a.b can be as "phi and masked"
	prog := Parse(`a = {}
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
`).Show()

	prog.Ref("c").ForEach(func(value *Value) {
		value.GetTopDefs().ForEach(func(value *Value) {
			t.Log(value.String())
		})
	})
}
