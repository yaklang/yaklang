package funcdefs

import "testing"

func TestCDefinitionsAreConsistent(t *testing.T) {
	for _, definition := range All() {
		if definition.Name == "" {
			t.Fatal("definition without a name")
		}
		if stored, ok := Lookup(definition.Name); !ok || stored.Name != definition.Name {
			t.Fatalf("lookup mismatch for %q", definition.Name)
		}
		for _, param := range definition.Params {
			if param.Name == "" {
				t.Fatalf("%s has an unnamed parameter", definition.Name)
			}
			if param.Type.CType() == "" {
				t.Fatalf("%s parameter %s has no C type", definition.Name, param.Name)
			}
		}
		if definition.Return.CType() == "" {
			t.Fatalf("%s has no return type", definition.Name)
		}
	}
}

func TestKnownSignaturesMatchCLibrary(t *testing.T) {
	cases := map[string]struct {
		params   []string
		returnID Kind
	}{
		"strncpy":  {[]string{"destination", "source", "num"}, Pointer},
		"memcmp":   {[]string{"left", "right", "num"}, Int},
		"snprintf": {[]string{"buffer", "size", "format"}, Int},
		"getline":  {[]string{"lineptr", "n", "stream"}, Int},
	}
	for name, want := range cases {
		definition, ok := Lookup(name)
		if !ok {
			t.Fatalf("%s is missing", name)
		}
		if len(definition.Params) != len(want.params) {
			t.Fatalf("%s has %d params", name, len(definition.Params))
		}
		for index, paramName := range want.params {
			if definition.Params[index].Name != paramName {
				t.Fatalf("%s param %d is %q", name, index, definition.Params[index].Name)
			}
		}
		if definition.Return.Kind != want.returnID {
			t.Fatalf("%s returns %v", name, definition.Return.Kind)
		}
	}
	if _, ok := Lookup("__libc_start_main"); !ok {
		t.Fatal("runtime entry is missing")
	}
}
