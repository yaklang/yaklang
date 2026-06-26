package types

import (
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
)

func TestParseSignature(t *testing.T) {
	ctx := &class_context.ClassContext{}
	tests := []struct {
		sig  string
		want string
	}{
		{"Ljava/util/function/BiFunction<Ljava/lang/Integer;Ljava/lang/Integer;Ljava/lang/Integer;>;", "BiFunction<Integer, Integer, Integer>"},
		{"Ljava/util/List<Ljava/lang/String;>;", "List<String>"},
		{"Ljava/util/Map<Ljava/lang/String;Ljava/lang/Integer;>;", "Map<String, Integer>"},
		{"Ljava/lang/String;", "String"},
		{"Ljava/util/List;", "List"},
		{"[Ljava/lang/String;", "String[]"},
	}
	for _, tt := range tests {
		typ := ParseSignature(tt.sig)
		if typ == nil {
			t.Errorf("ParseSignature(%q) returned nil", tt.sig)
			continue
		}
		got := typ.String(ctx)
		if got != tt.want {
			t.Errorf("ParseSignature(%q) = %q, want %q", tt.sig, got, tt.want)
		}
	}
}

func TestParseMethodSignature(t *testing.T) {
	ctx := &class_context.ClassContext{}
	sig := "(Ljava/util/function/BiFunction<Ljava/lang/Integer;Ljava/lang/Integer;Ljava/lang/Integer;>;)V"
	params, ret := ParseMethodSignature(sig)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	got := params[0].String(ctx)
	want := "BiFunction<Integer, Integer, Integer>"
	if got != want {
		t.Errorf("param type = %q, want %q", got, want)
	}
	if ret == nil {
		t.Fatal("return type is nil")
	}
	if ret.String(ctx) != "void" {
		t.Errorf("return type = %q, want void", ret.String(ctx))
	}
}
