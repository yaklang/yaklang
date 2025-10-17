package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBasic_Variable_Inblock(t *testing.T) {
	t.Run("test simple assign", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    println(a);
    a = 2;
    println(a);
    return 0;
}
	`, []string{
			"1",
			"2",
		}, t)
	})

	t.Run("test sub-scope capture parent scope", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    println(a);
    {
        a = 2;
        println(a);
    }
    println(a);
    return 0;
}
		`, []string{
			"1",
			"2",
			"2",
		}, t)
	})

	t.Run("test sub-scope local variable", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    println(a);
    {
        int a = 2;
        println(a);
    }
    println(a);
    return 0;
}
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})
}

func TestBasic_Variable_InIf(t *testing.T) {
	t.Run("test simple if", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    println(a);
    if (1) {
        a = 2;
        println(a);
    }
    println(a);
    return 0;
}
		`, []string{
			"1",
			"2",
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("test simple if else", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    println(a);
    if (1) {
        a = 2;
        println(a);
    } else {
        a = 3;
        println(a);
    }
    println(a);
    return 0;
}
		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[2,3]",
		}, t)
	})
}

func TestBasic_Variable_Loop(t *testing.T) {
	t.Run("simple loop not change", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    for (int i = 0; i < 10; i++) {
        println(a);
    }
    println(a);
    return 0;
}
		`,
			[]string{
				"1",
				"1",
			},
			t)
	})

	t.Run("simple loop", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int i = 0;
    for (i = 0; i < 10; i++) {
        println(i);
    }
    println(i);
    return 0;
}
		`,
			[]string{
				"phi(i)[0,add(i, 1)]",
				"phi(i)[0,add(i, 1)]",
			}, t)
	})
}

func TestBasic_Variable_Switch(t *testing.T) {
	t.Run("simple switch", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    switch (a) {
    case 2:
        a = 22;
        println(a);
        break;
    case 3:
    case 4:
        a = 33;
        println(a);
        break;
    default:
        a = 44;
        println(a);
        break;
    }
    println(a);
    return 0;
}
		`, []string{
			"22", "33", "44", "phi(a)[22,33,44]",
		}, t)
	})
}

func TestBasic_CFG_Break(t *testing.T) {
	t.Run("simple break in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    for (int i = 0; i < 10; i++) {
        if (i == 5) {
            a = 2;
            break;
        }
    }
    println(a);
    return 0;
}
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})
}

func TestBasic_CFG_Goto(t *testing.T) {
	t.Run("goto down in if", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    if (a > 1) {
        a = 5;
        goto end;
    } else {
        println(a);
    }
end:
    println(a);
    return 0;
}
		`, []string{
			"1", "phi(a)[5,1]",
		}, t)
	})

	t.Run("goto down in if and else", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1;
    if (a > 1) {
        a = 5;
        goto end;
    } else {
		println(a); 
end:
        println(a);
    }
	println(a);
    return 0;
}
		`, []string{
			"1", "phi(a)[1,5]", "phi(a)[5,phi(a)[1,5]]",
		}, t)
	})
}
