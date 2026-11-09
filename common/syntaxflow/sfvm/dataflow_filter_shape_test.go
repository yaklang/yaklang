package sfvm

import "testing"

func TestParseDataflowFilterShape(t *testing.T) {
	tests := []struct {
		name string
		text string
		kind DataflowFilterShapeKind
		sym  string
		ok   bool
	}{
		{"include any", "* & $source", DataflowFilterShapeAny, "source", true},
		{"include self", "<self> & $source", DataflowFilterShapeSelf, "source", true},
		{"include whitespace", "  *  &  $source  ", DataflowFilterShapeAny, "source", true},
		{"include repeated symbol", "* & $source & $source", DataflowFilterShapeAny, "source", true},
		{"include alias", "* & $params as $__next__", DataflowFilterShapeAny, "params", true},
		{"exclude callee", "*<getCallee> & $filter", DataflowFilterShapeCallee, "filter", true},
		{"exclude callee alias", "*<getCallee> & $filter as $x", DataflowFilterShapeCallee, "filter", true},
		{"exclude callee repeated", "*<getCallee> & $f & $f", DataflowFilterShapeCallee, "f", true},

		{"different symbols", "* & $a & $b", 0, "", false},
		{"self plus callee", "<self><getCallee> & $x", 0, "", false},
		{"self then intersection", "<self> & $x<getCallee>", 0, "", false},
		{"optional filter", "*?{opcode: call} as $x", 0, "", false},
		{"multi statement", "* & $x; $x --> $y", 0, "", false},
		{"ref variable first", "$x --> $y", 0, "", false},
		{"callee without symbol", "*<getCallee>", 0, "", false},
		{"bare symbol", "$source", 0, "", false},
		{"empty", "", 0, "", false},
		{"not a filter", "this is not syntax flow", 0, "", false},
		{"syntax error", "* & $x & *", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shape, ok := ParseDataflowFilterShape(tt.text)
			if ok != tt.ok {
				t.Fatalf("ParseDataflowFilterShape(%q) ok = %v, want %v", tt.text, ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if shape.Kind != tt.kind || shape.Symbol != tt.sym {
				t.Fatalf("ParseDataflowFilterShape(%q) = {%v %q}, want {%v %q}",
					tt.text, shape.Kind, shape.Symbol, tt.kind, tt.sym)
			}
		})
	}
}
