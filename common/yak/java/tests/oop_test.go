package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
	"testing"
)

func TestJava_OOP_Var_member(t *testing.T) {

	t.Run("test simple member call", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
class A {
		int a = 0;

}
class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a);
		}
}
		`, []string{"0"}, t)
	})

	t.Run("side effect", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	public class A {
		private int a = 0; 
	
		public void Hello(int a) { 
			println(a);
		}
}
		class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.Hello());
		}
}
		`, []string{
			"0", "side-effect(Parameter-$par, $this.a)",
		}, t)
	})
}
