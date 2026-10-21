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

	t.Run("exponential tree keeps real zero values on the bounded prefix", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("struct L0 { int A; int B; };\n")
		for i := 1; i <= 20; i++ {
			fmt.Fprintf(&b, "struct L%d { struct L%d X; struct L%d Y; };\n", i, i-1, i-1)
		}
		chain := "v" + strings.Repeat(".X", 20) + ".A"
		b.WriteString("int main() { struct L20 v; println(" + chain + "); }")
		// the leftmost chain is built first and stays within the node limit,
		// so it keeps the real zero value instead of a placeholder
		test.CheckPrintlnValueContain(b.String(), []string{"0"}, t)
	})

	t.Run("exponential tree degrades past the node limit", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("struct L0 { int A; int B; };\n")
		for i := 1; i <= 20; i++ {
			fmt.Fprintf(&b, "struct L%d { struct L%d X; struct L%d Y; };\n", i, i-1, i-1)
		}
		chain := "v" + strings.Repeat(".Y", 20) + ".A"
		b.WriteString("int main() { struct L20 v; println(" + chain + "); }")
		// the all-Y path is built after the left subtree has exhausted the
		// node limit, so it degrades to a placeholder instead of a zero value
		test.CheckPrintlnValueContain(b.String(), []string{"Undefined"}, t)
	})
}
