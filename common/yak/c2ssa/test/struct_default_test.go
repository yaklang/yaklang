package test

import (
	"fmt"
	"strings"
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// Test_Struct_Default_Expansion_Pruned mirrors the go2ssa struct default
// expansion guard for C. By-value diamond nesting is legal C (struct L1 holds
// two struct L0 fields, L2 two L1 ... -> 2^N nodes), and GetDefaultValue
// expanded that tree without dedup, so ~20 levels never finished compiling.
// Types whose expansion exceeds ssa.StructDefaultExpansionBudget degrade to
// a typed Make placeholder at the GetDefaultValue entry, so deeper reads
// resolve lazily while small structs keep exact zero-init (pointer members
// nil, scalars 0).
func Test_Struct_Default_Expansion_Pruned(t *testing.T) {
	t.Run("small struct keeps exact zero-value members", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

struct Leaf {
	int n;
	int m;
};
struct Small {
	struct Leaf a;
	struct Leaf b;
};

int main() {
	struct Small o;
	println(o.a.n);
	println(o.b.m);
}
			`, []string{"0", "0"}, t)
	})

	t.Run("exponential struct field tree compiles and stays readable", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("struct L0 { int A; int B; };\n")
		for i := 1; i <= 20; i++ {
			fmt.Fprintf(&b, "struct L%d { struct L%d X; struct L%d Y; };\n", i, i-1, i-1)
		}
		chain := "v" + strings.Repeat(".X", 20) + ".A"
		b.WriteString("int main() { struct L20 v; println(" + chain + "); }")
		// valid deep chain: v.X.X. ... .X.A (20 selects) resolves lazily
		test.CheckPrintlnValueContain(b.String(), []string{"Undefined"}, t)
	})
}
