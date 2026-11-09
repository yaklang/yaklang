package ssaapi

import "testing"

func TestClassifyDataflowFilterShape(t *testing.T) {
	tests := []struct {
		name string
		text string
		kind dataflowFilterKind
		sym  string
		ok   bool
	}{
		{"include any", "* & $source", dataflowFilterAny, "source", true},
		{"include self", "<self> & $source", dataflowFilterSelf, "source", true},
		{"include no spaces", "*&$source", dataflowFilterAny, "source", true},
		{"include whitespace", "  *  &  $source  ", dataflowFilterAny, "source", true},
		{"include newlines", "\n *\n &\n $source\n", dataflowFilterAny, "source", true},
		{"include repeated symbol", "* & $source & $source", dataflowFilterAny, "source", true},
		{"include alias", "* & $params as $__next__", dataflowFilterAny, "params", true},
		{"include repeated then alias", "* & $s & $s as $next", dataflowFilterAny, "s", true},
		{"exclude callee", "*<getCallee> & $filter", dataflowFilterCallee, "filter", true},
		{"exclude callee alias", "*<getCallee> & $filter as $x", dataflowFilterCallee, "filter", true},
		{"exclude callee repeated", "*<getCallee> & $f & $f", dataflowFilterCallee, "f", true},

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
		{"trailing junk", "* & $x & *", 0, "", false},
		{"alias then extra", "* & $x as $y & $x", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, ok := dataflowFilterShape(tt.text)
			if ok != tt.ok {
				t.Fatalf("dataflowFilterShape(%q) ok = %v, want %v", tt.text, ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if plan.kind != tt.kind || plan.symbol != tt.sym {
				t.Fatalf("dataflowFilterShape(%q) = {%v %q}, want {%v %q}",
					tt.text, plan.kind, plan.symbol, tt.kind, tt.sym)
			}
		})
	}
}
