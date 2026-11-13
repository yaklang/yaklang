package symbol

import "testing"

func TestAtPrefersExactThenSizedRange(t *testing.T) {
	symbols := []Symbol{
		{Name: "start", Address: 0x1000, Size: 0x20, Kind: SymbolFunction},
		{Name: "main", Address: 0x1010, Kind: SymbolFunction},
		{Name: "puts", Address: 0x2000, Kind: SymbolFunction, Imported: true},
	}
	if got, ok := At(symbols, 0x1010); !ok || got.Name != "main" {
		t.Fatalf("exact = %+v %v", got, ok)
	}
	if got, ok := At(symbols, 0x1004); !ok || got.Name != "start" {
		t.Fatalf("range = %+v %v", got, ok)
	}
	if _, ok := At(symbols, 0x3000); ok {
		t.Fatal("unknown address matched")
	}
}

func TestLookupReturnsBuiltinSignature(t *testing.T) {
	definition, ok := Lookup("memcpy")
	if !ok || len(definition.Params) != 3 || definition.Params[0].Name != "destination" {
		t.Fatalf("memcpy = %+v %v", definition, ok)
	}
	if _, ok := Lookup("definitely_not_a_libc_function"); ok {
		t.Fatal("unknown signature matched")
	}
}
