package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPointerSideEffectBoundaryCases(t *testing.T) {
	t.Run("positive branch writes expected side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void modify(int* a, int cond) {
	if (cond > 0) {
		*a = 7;
	} else {
		*a = 9;
	}
}

int main() {
	int x = 1;
	modify(&x, 1);
	println(x);
}
`, []string{"side-effect(phi(x)[7,9], x)"}, t)
	})

	t.Run("zero branch writes expected side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void modify(int* a, int cond) {
	if (cond > 0) {
		*a = 7;
	} else {
		*a = 9;
	}
}

int main() {
	int x = 1;
	modify(&x, 0);
	println(x);
}
`, []string{"side-effect(phi(x)[7,9], x)"}, t)
	})
}
