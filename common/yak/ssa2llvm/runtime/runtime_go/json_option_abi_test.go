package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/aotlib"
)

// roundTripABI sends a Go value through the same word encoding a Yak call
// return uses, then decodes it as an interface{} argument.
func roundTripABI(v any) (any, error) {
	word := uint64(runtimeValueToInt64(reflect.ValueOf(v)))
	decoded, err := runtimeDecodeArg(word, reflect.TypeOf((*any)(nil)).Elem())
	if err != nil || !decoded.IsValid() {
		return nil, err
	}
	return decoded.Interface(), nil
}

func TestJSONDumpOptionsSurviveABI(t *testing.T) {
	indent, err := roundTripABI(aotlib.JsonWithIndent("    "))
	if err != nil {
		t.Fatal(err)
	}
	got := aotlib.JsonDumps(map[string]any{"a": 1}, indent)
	want := "{\n    \"a\": 1\n}"
	if got != want {
		t.Fatalf("withIndent round trip:\n got %q\nwant %q", got, want)
	}

	compact, err := roundTripABI(aotlib.JsonWithIndent(""))
	if err != nil {
		t.Fatal(err)
	}
	got = aotlib.JsonDumps(map[string]any{"a": 1}, compact)
	if got != `{"a":1}` {
		t.Fatalf("empty indent round trip: got %q", got)
	}

	noEscape, err := roundTripABI(aotlib.JsonNoEscapeHTML())
	if err != nil {
		t.Fatal(err)
	}
	got = aotlib.JsonDumps(map[string]any{"url": "a&b"}, noEscape, compact)
	if strings.Contains(got, `\u0026`) || !strings.Contains(got, "&") {
		t.Fatalf("noEscapeHTML round trip: got %q", got)
	}
}
