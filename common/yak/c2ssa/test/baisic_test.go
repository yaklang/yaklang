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

		`, []string{"Function-add"}, t)
	})
}
