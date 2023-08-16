package dap

type FooBar struct {
	Baz int
	Bur string
}

// same member names, different order / types
type FooBar2 struct {
	Bur int
	Baz string
}

type Nest struct {
	Level int
	Nest  *Nest
}

var TestExtraLibs = make(map[string]interface{})

func init() {
	TestExtraLibs["get_a6"] = func() FooBar {
		return FooBar{Baz: 8, Bur: "word"}
	}
	TestExtraLibs["get_a7"] = func() *FooBar {
		return &FooBar{Baz: 5, Bur: "strum"}
	}
	TestExtraLibs["get_a8"] = func() FooBar2 {
		return FooBar2{Bur: 10, Baz: "feh"}
	}
	TestExtraLibs["get_a9"] = func() *FooBar {
		return (*FooBar)(nil)
	}
	TestExtraLibs["get_a11"] = func() [3]FooBar {
		return [3]FooBar{{1, "a"}, {2, "b"}, {3, "c"}}
	}
	TestExtraLibs["get_a12"] = func() []FooBar {
		return []FooBar{{4, "d"}, {5, "e"}}
	}
	TestExtraLibs["get_a13"] = func() []*FooBar {
		return []*FooBar{{6, "f"}, {7, "g"}, {8, "h"}}
	}
	TestExtraLibs["get_ms"] = func() Nest {
		return Nest{0, &Nest{1, &Nest{2, &Nest{3, &Nest{4, nil}}}}}
	}
}
