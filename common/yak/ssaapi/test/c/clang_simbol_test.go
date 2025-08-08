package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Express(t *testing.T) {
	t.Run("add", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    int a = 1 + 2;
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		a #-> as $target
		`, map[string][]string{
			"target": {"1", "2"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})

	t.Run("pointer", func(t *testing.T) {
		t.Skip()
		code := `
#include <stdio.h>
int main() {
    int a = 10;
    int *p = &a;
    int b = *p;
    return 0;
}
`
		ssatest.CheckSyntaxFlowEx(t, code, `
		p #-> as $target
		`, true, map[string][]string{
			"target": {"Undefined-b", "make(Pointer)"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})

	t.Run("struct", func(t *testing.T) {
		code := `
#include <stdio.h>
struct S { int x; int y; };
int main() {
    struct S s;
    s.x = 1;
    s.y = 2;
    return 0;
}
`
		ssatest.CheckSyntaxFlowEx(t, code, `
		s.x #-> as $target
		`, true, map[string][]string{
			"target": {"1"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_Stmt(t *testing.T) {
	code := `
#include <stdio.h>
int add(int a, int b) { return a + b; }
int main() {
    int f = add(1, 2);
    
    for (int i = 0; i < 3; i++) {
        f = add(f, i);
    }
    
    int a = f;
    return 0;
}
`

	ssatest.CheckSyntaxFlowContain(t, code, `
		a #-> as $a
		`,
		map[string][]string{
			"a": {"phi(a)[Function-add(1,2),Function-add(a,phi(i)[0,add(i, 1)])]", "phi(i)[0,add(i, 1)]"},
		}, ssaapi.WithLanguage(ssaapi.C),
	)
}

func Test_Function(t *testing.T) {
	t.Run("function call", func(t *testing.T) {
		code := `
#include <stdio.h>
int add(int a, int b) { return a + b; }
int main() {
    int result = add(3, 4);
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		add(* #-> as $target)
		`, map[string][]string{
			"target": {"3", "4"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})

	t.Run("recursive", func(t *testing.T) {
		code := `
#include <stdio.h>
int fact(int n) { 
    if (n <= 1) return 1; 
    return n * fact(n-2); 
}
int main() {
    int result = fact(5);
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		fact(* #-> as $target)
		`, map[string][]string{
			"target": {"5", "2", "1"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_ControlFlow(t *testing.T) {
	t.Run("if else", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    int a = 10;
    int b;
    if (a > 5) {
        b = 1;
    } else {
        b = 2;
    }
	int c = b;
    return 0;
}
`
		ssatest.CheckSyntaxFlowEx(t, code, `
		c #-> as $target
		`, true, map[string][]string{
			"target": {"1", "2"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})

	t.Run("while loop", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    int i = 0;
    int sum = 0;
    while (i < 3) {
        sum += i;
        i++;
    }
    return 0;
}
`
		ssatest.CheckSyntaxFlowEx(t, code, `
		sum #-> as $target
		`, true, map[string][]string{
			"target": {"0", "phi(i)[0,add(i, 1)]"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_Array(t *testing.T) {
	t.Run("array access", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    int arr[3] = {1, 2, 3};
    int x = arr[0];
    int y = arr[1];
	int z = arr[2];
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		x #-> as $x
		y #-> as $y
		z #-> as $z
		`, map[string][]string{
			"x": {"1"},
			"y": {"2"},
			"z": {"3"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_Pointer(t *testing.T) {
	t.Skip()
	t.Run("pointer arithmetic", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    int arr[3] = {1, 2, 3};
    int *p = arr;
    int x = *(p + 1);
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		p #-> as $target
		`, map[string][]string{
			"target": {"p", "1"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_Struct(t *testing.T) {
	t.Run("struct member", func(t *testing.T) {
		code := `
#include <stdio.h>
struct Point { int x; int y; };
int main() {
    struct Point p;
    p.x = 10;
    p.y = 20;
    int sum = p.x + p.y;
    return 0;
}
`
		ssatest.CheckSyntaxFlowEx(t, code, `
		sum #-> as $target
		`, true, map[string][]string{
			"target": {"10", "20"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_TypeCast(t *testing.T) {
	t.Run("type cast", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    double d = 3.14;
    int i = (int)d;
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		d #-> as $target
		`, map[string][]string{
			"target": {"3.14"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_Bitwise(t *testing.T) {
	t.Run("bitwise operations", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    int a = 6 & 3;
    int b = 6 | 3;
    int c = 6 ^ 3;
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		a #-> as $target
		`, map[string][]string{
			"target": {"6", "3"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}

func Test_CompoundAssign(t *testing.T) {
	t.Run("compound assignment", func(t *testing.T) {
		code := `
#include <stdio.h>
int main() {
    int a = 1;
    a += 2;
    a *= 3;
    return 0;
}
`
		ssatest.CheckSyntaxFlow(t, code, `
		a #-> as $target
		`, map[string][]string{
			"target": {"1", "2", "3"},
		}, ssaapi.WithLanguage(ssaapi.C))
	})
}
