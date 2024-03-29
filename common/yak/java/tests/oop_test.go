package tests

import "testing"

func TestJava_OOP_Var_member(t *testing.T) {

	t.Run("test simple member call", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
class A {
		int a = 0;
}
class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.a);
		}
}
		`, []string{"0"}, t)
	})
}
