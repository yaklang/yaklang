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
// Types whose expansion exceeds the node/depth limits degrade to
// a typed Make placeholder inside GetDefaultValue, so deeper reads
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

	t.Run("deep linear nesting degrades past the depth limit", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("struct L0 { int A; };\n")
		for i := 1; i <= 80; i++ {
			fmt.Fprintf(&b, "struct L%d { struct L%d F; int A; };\n", i, i-1)
		}
		shallow := "v" + strings.Repeat(".F", 63) + ".A"
		deep := "v" + strings.Repeat(".F", 75) + ".A"
		b.WriteString("int main() { struct L80 v; println(" + shallow + "); println(" + deep + "); }")
		// 63 nesting levels stay within defaultValueDepthLimit and keep the
		// real zero; 75 levels degrade to a placeholder
		test.CheckPrintlnValueContain(b.String(), []string{"0", "Undefined"}, t)
	})

	t.Run("exponential tree degrades past the node limit", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("struct L0 { int A; int B; };\n")
		for i := 1; i <= 10; i++ {
			fmt.Fprintf(&b, "struct L%d { struct L%d X; struct L%d Y; };\n", i, i-1, i-1)
		}
		left := "v" + strings.Repeat(".X", 10) + ".A"
		right := "v" + strings.Repeat(".Y", 10) + ".A"
		b.WriteString("int main() { struct L10 v; println(" + left + "); println(" + right + "); }")
		// the raw field tree holds 2^11-1 = 2047 struct nodes, past the
		// 1024-node budget: the leftmost chain is built first and keeps the
		// real zero value, the all-Y path degrades to a placeholder
		test.CheckPrintlnValueContain(b.String(), []string{"0", "Undefined"}, t)
	})
}
