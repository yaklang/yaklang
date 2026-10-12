package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// Test_Pointer_Defaults pins the zero-init struct defaults from
// GetDefaultValue: a struct with a pointer member zero-inits the member to
// nil rather than the integer 0, while scalar members default to 0.
// A bare `int *p;` stays valueless by design (see buildDeclarator's pointer
// comment) and is not asserted here.
func Test_Pointer_Defaults(t *testing.T) {
	t.Run("struct pointer member zero-init", func(t *testing.T) {
		// The pointer member defaults to nil (not the integer 0) while the
		// scalar member defaults to 0; the const folder does not fold
		// nil-vs-number comparisons, so the comparison stays symbolic.
		test.CheckPrintlnValue(`#include <stdio.h>

struct T {
	int* n;
	int v;
};

int main() {
	struct T o;
	println(o.n == 0);
	println(o.v);
}
			`, []string{"eq(nil, 0)", "0"}, t)
	})
}
