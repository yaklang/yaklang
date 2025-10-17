package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Pointer_normal(t *testing.T) {
	t.Run("basic pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main() {        
	int a = 1;
	int *p = &a;
	
	*p = 2;
	println(a);
	println(*p);
}
		`, []string{"2", "2"}, t)
	})

	t.Run("basic pointer overwrite", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main() {     
	int a = 1;
	int b = 2;
	int* p;

	p = &a;
	*p = 3;
	p = &b;
	*p = 4;
	p = &a;

	println(a);	// 3
	println(b);	// 4
	println(*p); // 3
}
		`, []string{"3", "4", "3"}, t)
	})

	t.Run("object pointer overwrite", func(t *testing.T) {

		test.CheckPrintlnValue(`#include <stdio.h>

struct T {
	int* n; 
};

int main(){
	int a = 1;
	int b = 2;

	struct T o1 = { .n = &a };
	struct T o2 = { .n = &a };

	*o1.n = 3;
	println(*o1.n);  // 3
	println(*o2.n);	// 3

	o2.n = &b;
	*o2.n = 4;
	println(*o1.n); // 3
	println(*o2.n); // 4
}	
		`, []string{"3", "3", "3", "4"}, t)
	})

	t.Run("struct pointer", func(t *testing.T) {

		test.CheckPrintlnValue(`#include <stdio.h>

struct T {
	int* n;
};

int main(){
	int a = 1;
	int b = 2;

	struct T s = { .n = &a };
	struct T* sp = &(struct T){ .n = &a };

	*s.n = 3;
	println(*s.n);  // 3
	println(*sp->n);	// 3

	sp->n = &b;
	*sp->n = 4;
	println(*s.n); // 3
	println(*sp->n); // 4
}
			
		`, []string{"3", "3", "3", "4"}, t)
	})

	t.Run("struct pointer overwrite", func(t *testing.T) {

		test.CheckPrintlnValue(`#include <stdio.h>

struct T {
	int* n;
};

int main(){
	int a = 1;
	int b = 2;

	struct T s1 = { .n = &a };
	struct T s2 = { .n = &a };
	struct T* sp = &s1;

	*sp->n = 3;
	println(*s1.n); // 3
	println(*s2.n); // 3

	sp->n = &b;
	*sp->n = 4;
	println(*s1.n); // 4
	println(*s2.n); // 3

	println(a); // 3
	println(b); // 4
}
			
		`, []string{"3", "3", "4", "3", "3", "4"}, t)
	})

	t.Run("same const reused by multiple variableMemories", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

struct A {
	int a; 
};

int main() {
	int n1 = 1;
	struct A str = { .a = n1 };

	int* p = &n1;
	*p = 2;

	println(str.a);	// 1
	println(n1);		// 2
}

		`, []string{"1", "2"}, t)

		test.CheckPrintlnValue(`#include <stdio.h>

struct A {
	int* a; 
};

int main() {
	int n1 = 1;
	struct A str = { .a = &n1 };

	int* p = &n1;
	*p = 2;

	println(*str.a);	// 2
	println(n1);		// 2
}

		`, []string{"2", "2"}, t)
	})

	t.Run("alias pointer", func(t *testing.T) {

		test.CheckPrintlnValue(`#include <stdio.h>

int add(int* a, int* b) {
	return *a + *b;
}

int main(){
	int a = 1;
	int b = 2;

	int c = add(&a, &b);
	println(a);
	println(b);
	println(c);
}
			
		`, []string{"1", "2", "Function-add(make(Pointer),make(Pointer))"}, t)
	})
}

func Test_Pointer_Muti(t *testing.T) {
	t.Run("basic muti pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p = &a;
	int** pp = &p;

	*p = 2;
	println(a); // 2
	**pp = 3;
	println(a); // 3
}
			
		`, []string{"2", "3"}, t)
	})

	t.Run("basic muti pointer overwrite", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int b = 2;

	int* p1 = &a;
	int* p2 = &b;

	int** pp = &p1;
	**pp = 3;
	println(a); // 3
	println(b); // 2

	pp = &p2;
	**pp = 4;
	println(a); // 3
	println(b); // 4
}
			
		`, []string{"3", "2", "3", "4"}, t)
	})
}

func Test_Pointer_Cfg(t *testing.T) {
	t.Run("pointer cfg block", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p = &a;
	{
		*p = 3;
		println(a);	// 3
		println(*p); // 3
	}
	println(a);	// 3
	println(*p); // 3
}

		`, []string{"3", "3", "3", "3"}, t)
	})

	t.Run("pointer cfg block local", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p = &a;
	{
		int a = 2;
		*p = 3;
		println(a);	// 2
		println(*p); // 3
	}
	println(a);	// 3
	println(*p); // 3
}

		`, []string{"2", "3", "3", "3"}, t)
	})

	t.Run("pointer cfg if", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p = &a;

	if (a > 0) {
		*p = 2;
	} else {
		*p = 3;
	}

	println(*p); // phi(p.@value)[2,3]
	println(a);	// phi(a)[2,3]
}
			
		`, []string{"phi(a)[2,3]", "phi(p.@value)[2,3]"}, t)
	})

	t.Run("pointer cfg if local", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p = &a;

	if (a > 0) {
		int a = 2;
		*p = 4;
	} else {
		int a = 3;
		*p = 5;
	}

	println(*p); // phi(p.@value)[4,5]
	println(a);	// phi(a)[4,5]
}
			
		`, []string{"phi(a)[4,5]", "phi(p.@value)[4,5]"}, t)
	})

	t.Run("pointer cfg if address", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int b = 2;
	int* p;

	if (a > b) {
		p = &a;
	} else {
		p = &b;
	}
	*p = 3;

	println(*p); // 3
	println(a);	// phi(a)[1,3]
	println(b);	// phi(b)[2,3]
}
			
		`, []string{"3", "phi(a)[1,3]", "phi(b)[2,3]"}, t)
	})

	t.Run("pointer cfg switch", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p;
	p = &a;

	switch (a) {
	case 1:
		*p = 2;
		break;
	case 2:
		*p = 3;
		break;
	}

	println(*p); // phi(p.@value)[2,3,1]
	println(a);	// phi(a)[2,3,1]
}
			
		`, []string{"phi(p.@value)[2,3,1]", "phi(a)[2,3,1]"}, t)
	})

	t.Run("pointer cfg switch address", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int b = 2;
	int* p;

	switch (a) {
	case 1:
		p = &a;
		break;
	case 2:
		p = &b;
		break;
	}
	*p = 3;

	println(*p); // 3
	println(a);	// phi(a)[1,3]
	println(b);	// phi(b)[2,3]
}
			
		`, []string{"3", "phi(a)[1,3]", "phi(b)[2,3]"}, t)
	})

	t.Run("pointer cfg for", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p;

	for (p = &a; ; ) {
		*p = 2; 
	}

	println(*p); // Undefined-p.@value
	println(a);	// phi(a)[1,2]
}
			
		`, []string{"Undefined-p.@value", "phi(a)[1,2]"}, t)
	})

	t.Run("pointer cfg for local", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int* p;

	for (p = &a; ; ) {
		int a = 2;
		*p = 2; 
	}

	println(*p); // Undefined-p.@value
	println(a);	// phi(a)[1,2]
}
			
		`, []string{"Undefined-p.@value", "phi(a)[1,2]"}, t)
	})

	t.Run("pointer cfg for address", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`#include <stdio.h>

int main(){
	int a = 1;
	int b = 2;
	int* p;

	for (p = &a; ; ) {
		p = &b;
	}
	*p = 3;

	println(*p); // 3
	println(a);	// phi(a)[1,3]
	println(b);	// phi(b)[2,3]
}
			
		`, []string{"3", "phi(a)[1,3]", "phi(b)[2,3]"}, t)
	})
}

func Test_Pointer_SideEffect(t *testing.T) {
	t.Run("pointer side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void pointer(int* a) {
	*a = 2;
}

int main() {
	int a = 1;

	println(a); // 1
	pointer(&a);
	println(a); // side-effect(2, a)
}

		`, []string{"1", "side-effect(2, a)"}, t)
	})

	t.Run("pointer side-effect cross block", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void pointer(int* a) {
	*a = 2;
}

int main() {
	int a = 1;
	{
		println(a); // 1
		pointer(&a);
		println(a); // side-effect(2, a)
	}
}

		`, []string{"1", "side-effect(2, a)"}, t)
	})

	t.Run("pointer side-effect cross block and local", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void pointer(int* a) {
	*a = 3;
}

int main() {
	int a = 1;
	{
		int a = 2;
		println(a); // 2
		pointer(&a);
		println(a); // side-effect(3, a)
	}
	println(a); // 1
	pointer(&a);
	println(a); // side-effect(3, a)
}

		`, []string{
			"2", "side-effect(3, a)",
			"1", "side-effect(3, a)"}, t)
	})

	t.Run("pointer side-effect with struct", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

struct Data {
	int value;
};

void modify_data(struct Data* d) {
	d->value = 42;
}

int main() {
	struct Data data = { .value = 10 };
	
	println(data.value); // 10
	modify_data(&data);
	println(data.value); // side-effect(42, data.value)
}

		`, []string{"10", "side-effect(42, data.value)"}, t)
	})

	// TODO
	t.Skip()

	t.Run("pointer side-effect with nested struct", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

struct Inner {
	int x;
};

struct Outer {
	struct Inner inner;
};

void modify_nested(struct Outer* outer) {
	outer->inner.x = 99;
}

int main() {
	struct Outer outer = { .inner = { .x = 5 } };
	
	println(outer.inner.x); // 5
	modify_nested(&outer);
	println(outer.inner.x); // side-effect(99, outer.inner.x)
}

		`, []string{"5", "side-effect(99, outer.inner.x)"}, t)
	})

	t.Run("pointer side-effect with array", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void modify_array(int* arr) {
	arr[0] = 100;
	arr[1] = 200;
}

int main() {
	int arr[3] = {1, 2, 3};
	
	println(arr[0]); // 1
	println(arr[1]); // 2
	modify_array(arr);
	println(arr[0]); // side-effect(FreeValue-arr, arr[0])
	println(arr[1]); // side-effect(FreeValue-arr, arr[1])
}

		`, []string{"1", "2", "side-effect(FreeValue-arr, arr[0])", "side-effect(FreeValue-arr, arr[1])"}, t)
	})

	t.Run("pointer side-effect with double pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void modify_through_double_pointer(int** pp) {
	**pp = 77;
}

int main() {
	int a = 10;
	int* p = &a;
	
	println(a); // 10
	modify_through_double_pointer(&p);
	println(a); // side-effect(77, a)
}

		`, []string{"10", "side-effect(77, a)"}, t)
	})

	t.Run("pointer side-effect multiple parameters", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void modify_multiple(int* a, int* b, int* c) {
	*a = 11;
	*b = 22;
	*c = 33;
}

int main() {
	int x = 1, y = 2, z = 3;
	
	println(x); // 1
	println(y); // 2
	println(z); // 3
	modify_multiple(&x, &y, &z);
	println(x); // side-effect(11, x)
	println(y); // side-effect(22, y)
	println(z); // side-effect(33, z)
}

		`, []string{
			"1", "2", "3",
			"side-effect(11, x)",
			"side-effect(22, y)",
			"side-effect(33, z)"}, t)
	})

	t.Run("pointer side-effect with conditional", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void conditional_modify(int* a, int condition) {
	if (condition > 0) {
		*a = 50;
	} else {
		*a = 60;
	}
}

int main() {
	int a = 1;
	
	println(a); // 1
	conditional_modify(&a, 1);
	println(a); // side-effect(phi(a)[50,60], a)
}

		`, []string{"1", "side-effect(phi(a)[50,60], a)"}, t)
	})
}

func Test_Pointer_SideEffect_Parameter(t *testing.T) {
	t.Run("parameter", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

void pointer(int* b, int c) {
	*b = c;
}

int main() {
	int a = 1;

	println(a); // 1
	pointer(&a, 3);
	println(a); // side-effect(3, a)
}

		`, []string{"1", "side-effect(3, a)"}, t)
	})

	t.Run("parameter with struct", func(t *testing.T) {
		test.CheckPrintlnValue(`#include <stdio.h>

struct Data {
	int value;
};

void modify_data(struct Data* d,int e) {
	d->value = e;
}

int main() {
	struct Data data = { .value = 10 };
	
	println(data.value); // 10
	modify_data(&data, 44);
	println(data.value); // side-effect(44, data.value)
}

		`, []string{"10", "side-effect(44, data.value)"}, t)
	})

	t.Run("lib-gets paramete", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

void vulnerable_file_list(const char *directory) {
    char command[256];
    gets(&command);
    println(command);
}
		`, []string{"side-effect(make(any), command)"}, t)
	})

	t.Run("lib-sprintf parameter", func(t *testing.T) {
		test.CheckPrintlnValue(`
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

void vulnerable_file_list(const char *directory) {
    char command[256];
    sprintf(command, "ls -la %s", directory);
    println(command);
}
		`, []string{"side-effect(Parameter-directory, command)"}, t)
	})

}
