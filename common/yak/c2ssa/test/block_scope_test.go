package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBlock_Normol(t *testing.T) {
	t.Run("cross block", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    {
        int a = 2;
        a = 4;
        println(a);
    }
    println(a);
    return 0;
}
		`, []string{
			"4", "1",
		}, t)
	})
}

func TestBlock_Value_If(t *testing.T) {
	t.Run("if with declaration", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 2;
    if (a > 1) {
        println(a);
        a = 3;
    } else {
        println(a);
        a = 4;
    }
    println(a);
    return 0;
}
		`, []string{
			"2", "2", "phi(a)[3,4]",
		}, t)
	})

	t.Run("if with outer variable", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    if (a > 0) {
        println(a);
    } else {
        println(a);
    }
    println(a);
    return 0;
}
		`, []string{
			"1", "1", "1",
		}, t)
	})
}

func TestBlock_Value_For(t *testing.T) {
	t.Run("for with declaration", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    for (int i = 1; i < 10; i++) {
        println(i);
    }
    return 0;
}
		`, []string{"phi(i)[1,add(i, 1)]"}, t)
	})

	t.Run("for with outer variable", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int i = 10;
    for (int i = 5; i < 10; i++) {
        println(i);
    }
    println(i);
    return 0;
}
		`, []string{"phi(i)[5,add(i, 1)]", "10"}, t)
	})
}

func TestBlock_Return_Phi(t *testing.T) {
	t.Run("phi-with-return", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    if (1) {
        return 0;
    }
    println(a);
    return 0;
}
		`, []string{"phi(a)[Undefined-a,1]"}, t)
	})
}
