package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBuild(t *testing.T) {
	t.Run("build", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main(int a,int b) {        
    println("Hello, World!\n");  
    return 0;        
}
		`, []string{`"Hello, World!\n"`}, t)
	})
}

// func TestBuild_Tmp(t *testing.T) {
// 	t.Run("build", func(t *testing.T) {
// 		test.CheckPrintlnValue(`
// #include <stdio.h>

// int main() {
// 	int a = 1;
// 	a++;
// 	int b = 1;
// 	++b;
// 	int c = 1;
// 	c += a;

// 	println(a);
// 	println(b);
// 	println(c);
// }
// 		`, []string{``}, t)
// 	})
// }

func TestExpr_normol(t *testing.T) {
	t.Run("add sub mul div", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {        
	int a = 10.0;
	int b = 5.0;
	
	int add = a + b;
	int sub = a - b;
	int mul = a * b;
	int div = a / b;
	
	println(add);
	println(sub);
	println(mul);
	println(div);
}
		`, []string{"15", "5", "50", "2"}, t)
	})

	t.Run("float", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {  
	int a = 10.0;
	int b = 4.0 + a;
	int c = b / a;

	println(a);
	println(b);
	println(c);
}
		`, []string{"10", "14", "1.4"}, t)
	})

	t.Run("assign add", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {  
	int a = 1;
	a++;
	int b = 1;
	++b;
	int c = 1;
	c += a;

	println(a);
	println(b);
	println(c);
}
		`, []string{"2", "2", "3"}, t)
	})
}

func TestFuntion_normol(t *testing.T) {
	t.Run("call", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int add(int a,int b){
	return a + b;
}

int main() {        
	int c = add(1, 2);
	println(c);
}

		`, []string{"Function-add(1,2)"}, t)
	})

	t.Run("simple return", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int add(int a, int b) { return a + b; }
int main() {
    int c = add(2, 3);
    println(c);
    return 0;
}
`, []string{"Function-add(2,3)"}, t)
	})

	t.Run("void function", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
void print_hello() { println("hello"); }
int main() {
    print_hello();
    return 0;
}
`, []string{`"hello"`}, t)
	})
}

func TestStmt_normol(t *testing.T) {
	t.Run("if expr", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {        
	int a;
	if (a > 1) {
		a = 7;
	}
	println(a);
}
		`, []string{"phi(a)[7,0]"}, t)
	})

	t.Run("if expr else", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {        
	int a;
	if (a == 1) {
		a = 6;
	} else {
		a = 7;
	}
	println(a);
}
		`, []string{"phi(a)[6,7]"}, t)
	})

	t.Run("switch exp case", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {        
	int a;
	switch (a) {
		case 2:
		a = 2;
		case 3:
		a = 3;
	}
	println(a);
}
		`, []string{"phi(a)[3,0]"}, t)
	})

	t.Run("switch exp case break", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {        
	int a;
	switch (a) {
		case 1:
			a = 1;
			break;
		case 2:
			a = 2;
			break;
		case 3:
			a = 3;
			break;
		default:
			a = 0;
	}
	println(a);
}
		`, []string{"phi(a)[1,2,3,0]"}, t)
	})

	t.Run("for exp", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {        
 	int i = 0;
	for ( ; i < 10; ) {
		i++;
	}
	println(i);
}
		`, []string{"phi(i)[0,add(i, 1)]"}, t)
	})

	t.Run("for stmt;exp;stmt", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>

int main() {        
 	int i = 0;
	for (i = 1; i < 10; i++) {
	}
	println(i);
}
		`, []string{"phi(i)[1,add(i, 1)]"}, t)
	})
}

func TestType_normol(t *testing.T) {
	t.Run("string concat", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    println("foo" "bar");
    println("hello, " "world!\n");
    return 0;
}
`, []string{`"foobar"`, `"hello, world!\n"`}, t)
	})

	t.Run("compound literal", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
struct S { int x; int y; };
int main() {
    struct S s = (struct S){.x=1, .y=2};
    println(s.x);
    println(s.y);
    return 0;
}
`, []string{"1", "2"}, t)
	})

	t.Run("conditional expr", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1, b = 2;
    int c = a > b ? a : b;
    println(c);
    return 0;
}
`, []string{"phi(c)[1,2]"}, t)
	})

	t.Run("bitwise and shift", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 6 & 3;
    int b = 6 | 3;
    int c = 6 ^ 3;
    int d = 1 << 3;
    int e = 8 >> 2;
    println(a); println(b); println(c); println(d); println(e);
    return 0;
}
`, []string{"2", "7", "5", "8", "2"}, t)
	})

	t.Run("compound assign", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1; a += 2; a *= 3; a -= 1; a /= 2; a %= 2;
    println(a);
    return 0;
}
`, []string{"0"}, t)
	})

	t.Run("inc dec", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 1; a++; ++a; a--; --a;
    println(a);
    return 0;
}
`, []string{"1"}, t)
	})

	t.Run("array", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int arr[3] = {1,2,3};
    println(arr[0]);
    println(arr[1]);
    println(arr[2]);
    return 0;
}
`, []string{"1", "2", "3"}, t)
	})

	t.Run("pointer", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`
#include <stdio.h>
int main() {
    int a = 10;
    int *p = &a;
    println(*p);
    return 0;
}
`, []string{"10"}, t)
	})

	t.Run("struct", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
struct S { int x; int y; };
int main() {
    struct S s; s.x = 5; s.y = 6;
    println(s.x);
    println(s.y);
    return 0;
}
`, []string{"5", "6"}, t)
	})

	t.Run("malloc", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
#include <stdlib.h>

struct Node {
    int data;
    struct Node* next;
};

int main() {
    struct Node* head = malloc(sizeof(struct Node));
    head->data = 10;
    println(head->data);
	println(head);
    return 0;
}
`, []string{"10", "Function-malloc"}, t)
	})
}
