package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestFunction_Value(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
void test() {
    int a = 1;
    println(a);
}
int main() {
    test();
    return 0;
}
		`, []string{
			"1",
		}, t)
	})

	t.Run("function call", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int add(int a, int b) {
    return a + b;
}
int main() {
    println(add);
    println(add(1, 2));
    return 0;
}
		`, []string{
			"Function-add", "Function-add(1,2)",
		}, t)
	})

	t.Run("function call forward declaration", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int add(int a, int b);
int main() {
    int c = add(1, 2);
    println(c);
    return 0;
}
int add(int a, int b) {
    return a + b;
}
		`, []string{
			"Function-add(1,2)",
		}, t)
	})
}

func TestFunction_GlobalValue(t *testing.T) {
	t.Skip()
	t.Run("global value", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
int main() {
    println(count);
    return 0;
}
		`, []string{
			"1",
		}, t)
	})

	t.Run("global value phi", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
int main() {
    if (1) {
        count = 2;
        println(count);
    }
    println(count);
    return 0;
}
		`, []string{
			"2", "phi(count)[2,1]",
		}, t)
	})

	t.Run("global value phi scope", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
int main() {
    if (1) {
        count = 2;
        count = 3;
        println(count);
    }
    println(count);
    return 0;
}
		`, []string{
			"3", "phi(count)[3,1]",
		}, t)
	})

	t.Run("global value phi scope sub", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
int main() {
    if (1) {
        count = 2;
        {
            count = 3;
        }
        println(count);
    }
    println(count);
    return 0;
}
		`, []string{
			"3", "phi(count)[3,1]",
		}, t)
	})

	t.Run("global value phi merge", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
int main() {
    count = 2;
    if (1) {
        count = 3;
    } else {
        count = 4;
    }
    println(count);
    return 0;
}
void main2() {
    println(count);
}
		`, []string{
			"phi(count)[3,4]", "1",
		}, t)
	})

	t.Run("global value phi mergeEX", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
int main() {
    if (1) {
        count = 3;
    } else {
        count = 4;
    }
    count = 5;
    println(count);
    return 0;
}
void main2() {
    println(count);
}
		`, []string{
			"5", "1",
		}, t)
	})

	t.Run("global value phi function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
void f() {
    count = 2;
    println(count);
}
int main() {
    println(count);
    if (1) {
        count = 3;
    } else {
        count = 4;
    }
    println(count);
    return 0;
}
		`, []string{
			"2", "1", "phi(count)[3,4]",
		}, t)
	})

	t.Run("global value phi function-if", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
void f1() {
    count = 2;
    println(count);
}
void f2() {
    if (1) {
        count = 3;
    }
    println(count);
}
int main() {
    println(count);
    return 0;
}
		`, []string{
			"1", "2", "phi(count)[3,1]",
		}, t)
	})

	t.Run("global value phi function-loop", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int count = 1;
void f1() {
    count = 2;
    println(count);
}
void f2() {
    for (count = 3; count > 0; count--) {
    }
    println(count);
}
int main() {
    println(count);
    return 0;
}
		`, []string{
			"2", "phi(count)[3,sub(count,1)]", "1",
		}, t)
	})
}

func TestFunction_Pointer(t *testing.T) {
	t.Skip()
	t.Run("function pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int add(int a, int b) {
    return a + b;
}
int main() {
    int (*func_ptr)(int, int) = add;
    println(func_ptr);
    println(func_ptr(1, 2));
    return 0;
}
		`, []string{
			"Function-add", "Function-add(1,2)",
		}, t)
	})

	t.Run("function pointer array", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int add(int a, int b) { return a + b; }
int sub(int a, int b) { return a - b; }
int mul(int a, int b) { return a * b; }
int main() {
    int (*funcs[3])(int, int) = {add, sub, mul};
    println(funcs[0](1, 2));
    println(funcs[1](5, 2));
    return 0;
}
		`, []string{
			"Function-add(1,2)", "Function-sub(5,2)",
		}, t)
	})
}

func TestFunction_Recursive(t *testing.T) {
	t.Run("recursive function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int factorial(int n) {
    if (n <= 1) return 1;
    return n * factorial(n - 1);
}
int main() {
    println(factorial(5));
    return 0;
}
		`, []string{
			"Function-factorial(5)",
		}, t)
	})

	t.Run("mutual recursive", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int is_even(int n);
int is_odd(int n) {
    if (n == 0) return 0;
    return is_even(n - 1);
}
int is_even(int n) {
    if (n == 0) return 1;
    return is_odd(n - 1);
}
int main() {
    println(is_even(4));
    return 0;
}
		`, []string{
			"Function-is_even(4)",
		}, t)
	})
}

func TestFunction_Variadic(t *testing.T) {
	t.Run("variadic function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
#include <stdarg.h>
int sum(int count, ...) {
    va_list args;
    va_start(args, count);
    int total = 0;
    for (int i = 0; i < count; i++) {
        total += va_arg(args, int);
    }
    va_end(args);
    return total;
}
int main() {
    println(sum(3, 1, 2, 3));
    return 0;
}
		`, []string{
			"Function-sum(3,1,2,3)",
		}, t)
	})
}

func TestFunction_Inline(t *testing.T) {
	t.Run("inline function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
static inline int square(int x) {
    return x * x;
}
int main() {
    println(square(5));
    return 0;
}
		`, []string{
			"Function-square(5)",
		}, t)
	})
}

func TestFunction_Static(t *testing.T) {
	t.Run("static function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
static int helper(int x) {
    return x * 2;
}
int main() {
    println(helper(3));
    return 0;
}
		`, []string{
			"Function-helper(3)",
		}, t)
	})
}

func TestFunction_Extern(t *testing.T) {
	t.Run("extern function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
extern int external_func(int x);
int main() {
    println(external_func(10));
    return 0;
}
		`, []string{
			"Function-external_func(10)",
		}, t)
	})
}
