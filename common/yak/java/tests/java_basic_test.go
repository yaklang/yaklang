package tests

import (
	"testing"
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
		`, []string{"make([]any)", "make([]number)", "\"hello\"", "3", "3", "make([]number)"}, t)
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
