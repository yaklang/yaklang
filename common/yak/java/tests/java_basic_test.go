package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJava_LocalType_Declaration(t *testing.T) {
	t.Run("test simple variable assign", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a;
		println(a);
		int a = 1;
		println(a);
		Boolen b= true;
		println(b);
		float c=3.14;
		println(c);
		string s ="aaa";
		println(s);`, []string{"Undefined-a", "1", "true", "3.14", "\"aaa\""}, t)
	})
	t.Run("test array declaration", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a[] = {};
		println(a);
		int a[] = {1,2,3};
		println(a);
	    string s[] = {"world","hello"};
		println(s[1]);
		println(a[2]);
		int c=a[1]+a[0];
		println(c);
		int[] numbers = {1,2,3};
		println(numbers);
		`, []string{"make([]number)", "make([]number)", "\"hello\"", "3", "3", "make([]number)"}, t)
	})
	t.Run("test two dim array declaration", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a[][] = {{1,2,3},{4,5,6}};
		println(a);
		println(a[1][2]);

		String a[][]={{"hello","world"},{"world","hello"}};
		println(a[1][1]);
		`,
			[]string{"make([][]number)",
				"6",
				"\"hello\""}, t)
	})
	t.Run("test array declaration", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		Object a[][][] = {{{1,2,3}}};
		println(a[0][0][0]);
		Object b[][] = {{1,2},3};
		println(b[0][1]);
		`,
			[]string{"1",
				"2",
			}, t)
	})

	t.Run("test return", func(t *testing.T) {
		CheckAllJavaCode(`
public class HelloWorld {
    public static void main(String[] args) {
        int result = a + b;
        return result;
        int a=2;
    }
}

`, t)
	})
	t.Run("test switch break", func(t *testing.T) {
		CheckJavaCode(`
		result= switch(e){
		default : break;
};
`, t)
	})
}

func TestJavaSyntaxBlock(t *testing.T) {
	t.Run("test simple block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class A{
	public static void main(String[] args){
		{
			int a=2;
			println(a); // 2 
		}
		println(a); //
	}
}
	`, []string{"2", "Undefined-a"}, t)
	})

	t.Run("test synchronized block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	class A{
		public static void main(String[] args){
			synchronized(this){
				println("hello");
			}
		}
	}`, []string{`"hello"`}, t)
	})
}

func TestJavaTernaryExpression(t *testing.T) {
	t.Run("test basic ternary", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String result = (a > 3) ? "greater" : "smaller";
		println(result);
		`, []string{`"greater"`}, t)
	})

	t.Run("test nested ternary", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String result = (a > 10) ? "large" : (a > 3) ? "medium" : "small";
		println(result);
		`, []string{`"medium"`}, t)
	})

	t.Run("test ternary with expressions", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		int b = 10;
		int result = (c) ? (a + b) : (b - a);
		println(result);
		`, []string{`phi(result)[15,5]`}, t)
	})

	t.Run("test ternary with boolean result", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		int b = 3;
		boolean result = (c) ? true : false;
		println(result);
		boolean result2 = (c) ? true : false;
		println(result2);
		`, []string{`phi(result)[true,false]`, `phi(result2)[true,false]`}, t)
	})

	t.Run("test ternary with method calls", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String str = "test";
		String result = (c) ? str.toUpperCase() : str.toLowerCase();
		println(result);
		`, []string{`phi(result)[Undefined-str.toUpperCase("test"),Undefined-str.toLowerCase("test")]`}, t)
	})

	// Test conditional branches in ternary expressions
	t.Run("test ternary condition evaluation", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String result = (c) ? "condition true" : "condition false";
		println(result);
		`, []string{`phi(result)["condition true","condition false"]`}, t)
	})
}
