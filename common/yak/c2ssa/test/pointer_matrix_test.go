package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPointerReferenceMatrix(t *testing.T) {
	t.Run("basic pointer write", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main() {
	int a = 1;
	int* p = &a;
	*p = 2;
	println(a);
}
`, []string{"2"}, t)
	})

	t.Run("boundary alias pointers same target", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void write_left(int* left) {
	*left = 7;
}

void write_right(int* right) {
	*right = 8;
}

int main() {
	int a = 1;
	int* p1 = &a;
	int* p2 = &a;
	write_left(p1);
	write_right(p2);
	println(a);
}
`, []string{"side-effect(8, a)"}, t)
	})

	t.Run("complex branch plus repeated writes", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void update(int* p, int flag) {
	if (flag > 0) {
		*p = 11;
	} else {
		*p = 12;
	}
}

int main() {
	int a = 1;
	update(&a, 1);
	update(&a, 0);
	println(a);
}
`, []string{"side-effect(phi(a)[11,12], a)"}, t)
	})

	t.Run("boundary pointer alias chain rebinding", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main() {
	int a = 1;
	int b = 2;
	int* p = &a;
	int* q = p;

	q = &b;
	*p = 3;
	*q = 4;

	println(a);
	println(b);
}
`, []string{"3", "4"}, t)
	})

	t.Run("complex alias chain branch and multi-call", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void update(int* p, int flag) {
	if (flag > 0) {
		*p = 13;
	} else {
		*p = 14;
	}
}

int main() {
	int a = 1;
	int* p = &a;
	int* q = p;
	update(q, 1);
	update(p, 0);
	println(a);
}
`, []string{"side-effect(phi(a)[13,14], a)"}, t)
	})
}
