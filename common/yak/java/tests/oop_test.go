package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
	"testing"
)

func TestJava_OOP_Var_member(t *testing.T) {

	t.Run("test simple member call", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
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

	t.Run("side effect", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	 class A {
		private int a = 0; 
	
		public void setA(int par) { 
			this.a = par;
		}
}
		class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.a);
			a.setA(1);
			println(a.a);
		}
}
		`, []string{
			"0", "side-effect(Parameter-par, this.a)",
		}, t)
	})

	t.Run("free-value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			int a = 0; 
			public void getA() {
				return this.a;
			}
		}

		class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getA());
		a.a=1;
		println(a.getA());
		}
}
		`, []string{
			"Function-.getA(make(object{}),0)",
			"Function-.getA(make(object{}),1)",
		}, t)
	})

	t.Run("just use method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	public 	class A {
			int a = 0; 
			public void getA(){
			return this.a;
			}
			
			public void setA(int par){
			this.a=par;
			}
		}
	public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getA());
		a.setA(1);
		println(a.getA());
		}
}
		`, []string{
			"Function-.getA(make(object{}),0)",
			"Function-.getA(make(object{}),side-effect(Parameter-par, this.a))",
		}, t)
	})
}

func TestJava_Extend_Class(t *testing.T) {

	t.Run("test extend constant ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		class A {
			int a = 0; 
		}
	public class B extends A{}
	public class C extends B{}
	public class Main{
		public static void main(String[] args) {
		C C = new C();
		println(C.a); // 0
}}
		`, []string{
			"0",
		}, t)
	})

	t.Run("free-value", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	public 	class Q {
			int a = 0; 
			public void getA() {
				return this.a;
			}
		}
		class A extends Q{}
		public class Main{
		public static void main(String[] args) {
			
		A a = new A(); 
		println(a.getA());
		a.a=1;
		println(a.getA());
		}
}
		`, []string{
			"Function-.getA(make(object{}),0)",
			"Function-.getA(make(object{}),1)",
		}, t)
	})

	t.Run("just use method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public class Q {
			int a = 0; 
			public void getA(){
			return this.a;
			}
			
			public void setA(int par){
			this.a=par;
			}
		}
		class A extends Q{}
		public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getA());
		a.setA(1);
		println(a.getA());
		}
}
		`, []string{
			"Function-.getA(make(object{}),0)",
			"Function-.getA(make(object{}),side-effect(Parameter-par, this.a))",
		}, t)
	})
}

func TestJava_Construct(t *testing.T) {
	t.Run("no construct", func(t *testing.T) {
		code := `
	public	class A {
			int num = 0;
			public int getNum() {
				super();
				return this.num;
			}
		}
public class Main{
		public static void main(String[] args) {
		A a = new A(); 
		println(a.getNum());
		}
}
		`
		ssatest.CheckPrintlnValue(code, []string{
			"Function-.getNum(make(object{}),0)",
		}, t)
	})

	t.Run("normal construct", func(t *testing.T) {
		code := `
public class A {
	private int  num = 0;
	public A(int num) {
		this.num = num;
	}
	public int getNum() {
		return this.num;
	}
}
public class Main{
		public static void main(String[] args) {
		A a = new A(1);
		println(a.getNum());
		}
}
`
		ssatest.CheckPrintlnValue(code, []string{
			"Function-.getNum(make(object{}),side-effect(Parameter-num, this.num))",
		}, t)
	})
}
