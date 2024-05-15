package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
			"Function-_A_getA(make(A)) member[0]",
			"Function-_A_getA(make(A)) member[1]",
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
			"Function-_A_getA(make(A)) member[0]",
			"Function-_A_getA(make(A)) member[side-effect(Parameter-par, this.a)]",
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
			"Function-_Q_getA(make(A)) member[0]",
			"Function-_Q_getA(make(A)) member[1]",
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
			"Function-_Q_getA(make(A)) member[0]",
			"Function-_Q_getA(make(A)) member[side-effect(Parameter-par, this.a)]",
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
			"Function-_A_getNum(make(A)) member[0]",
		}, t)
	})

	t.Run("normal construct", func(t *testing.T) {
		code := `
public class A {
	private int num1=0;
	private int num2=0;
	
	public A(int num1,int num2) {
		this.num1 = num1;
		this.num2 = num2;

	}
	public int getNum1() {
		return this.num1;
	}
	public int getNum2(){
	return this.num2;
}
}
public class Main{
		public static void main(String[] args) {
		A a = new A(1,2);
		println(a.getNum1());
		println(a.getNum2());
		}
}
`
		ssatest.CheckPrintlnValue(code, []string{
			"Function-_A_getNum1(make(A)) member[side-effect(Parameter-num1, this.num1)]",
			"Function-_A_getNum2(make(A)) member[side-effect(Parameter-num2, this.num2)]",
		}, t)
	})
}

func TestJava_OOP_Enum(t *testing.T) {
	t.Run("test simple top-level enum", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public enum A {
			A,B,C;
		}
		public class Main{
			public static void main(String[] args) {
			A a = A.B;
			println(a);
			}
		}
		`, []string{
			"make(A)",
		}, t)
	})

	t.Run("test  top-level enum with constructor", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
		public enum A {
			A(1,2),
			B(3,4),
			C(4,5);
			private final int num1;
			private final int num2;

			A(int par1,int par2){
				this.num1=par1;
				this.num2=par2;
			}

			public int getNum1(){
			return this.num1;
			}

			public int getNum2(){
			return this.num2;
			}
		}
		public class Main{
			public static void main(String[] args) {
			A a = A.B;
			println(a.getNum1());
			println(a.getNum2());
			}
		}
		`, []string{
			"Function-_A_getNum1(make(A)) member[side-effect(Parameter-par1, a.num1)]",
			"Function-_A_getNum2(make(A)) member[side-effect(Parameter-par2, a.num2)]",
		}, t)
	})

}

func TestJava_OOP_MemberClass(t *testing.T) {
	t.Run("test no-static inner class ", func(t *testing.T) {
		code := `
public class Outer {
    public  class Inner{
        int a = 1;
        public Inner(int par){
            this.a=par;
        }
        public int getA(){
            return this.a;
        }
    }
}

public class Main{
    public static void main(String[] args) {
        Outer outer = new Outer();
        Outer.Inner inner =outer.new Inner(5);
        println(inner);
		println(inner.getA());
    }
}`
		ssatest.CheckPrintlnValue(code, []string{
			"make(Outer.Inner)",
			"Function-_Outer.Inner_getA(make(Outer.Inner)) member[side-effect(Parameter-par, this.a)]",
		}, t)
	})
}

func TestJava_OOP_Static_Member(t *testing.T) {
	t.Run("test simple static member", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class A {
		static int a = 0;
}
class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.a);
		}
}
		`, []string{"Undefined-a.a(valid)"}, t)
	})

	t.Run("test static variable and static method within a class", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    static int a = 1 ;
    public static void main(String[] args) {
            println(a);
        }
 }
			`, []string{"1"}, t)
	})

	t.Run("test static variable and method within a class (arg is a)", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    static int a = 1 ;
    public void main(String[] args) {
           println(a);
        }
 }
			`, []string{"1"}, t)
	})

	t.Run("test static variable and  method within a class (arg is this.a)", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    static int a = 1 ;
    public void main(String[] args) {
            println(this.a);
        }
 }
			`, []string{"ParameterMember-parameter[0].a"}, t)
	})

	t.Run("test member variable and  method within a class ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    int a = 1 ;
    public void main(String[] args) {
            println(this.a);
        }
 }
			`, []string{"ParameterMember-parameter[0].a"}, t)
	})

	t.Run("test member variable and  method within a class ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    int a = 1 ;
    public void main(String[] args) {
            println(a);
        }
 }
			`, []string{"1"}, t)
	})

	t.Run("test member variable and static method within a class", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    int a = 1 ;
    public void main(String[] args) {
            println(a);
        }
 }
			`, []string{"1"}, t)
	})

	t.Run("test cross class static variable calls ", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
package org.example;
public class Test {
		static int a = 1;
	}

public class Main {
    public void main(String[] args) {
           println(Test.a);
        }
 }
	
			`, []string{"1"}, t)
	})

	t.Run("test static method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class A {
		int a = 0;
		public static void Hello(){
        }

}
class Main{
		public static void main(String[] args) {
			A a = new A();
			println(a.Hello());
		}
}
		`, []string{"Undefined-a.Hello(valid)()"}, t)
	})

	t.Run("test call self's static method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class Main{
		public static void Hello(){
        }
		public static void main(String[] args) {
			println(Hello());
		}
}
		`, []string{"Function-_Main_Hello()"}, t)
	})
}

func TestJava_Package(t *testing.T) {
	t.Run("simple test", func(t *testing.T) {
		code := `
	package org.example.A;
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
			"Function-org_example_A_A_getNum(make(A)) member[0]",
		}, t)
	})

	t.Run("test package with constructor", func(t *testing.T) {
		code := `
	package com.example.A;
	public class A {
	private int num1=0;
	private int num2=0;
	
	public A(int num1,int num2) {
		this.num1 = num1;
		this.num2 = num2;

	}
	public int getNum1() {
		return this.num1;
	}
	public int getNum2(){
	return this.num2;
}
}
public class Main{
		public static void main(String[] args) {
		A a = new A(1,2);
		println(a.getNum1());
		println(a.getNum2());
		}
}
		`
		ssatest.CheckPrintlnValue(code, []string{
			"Function-com_example_A_A_getNum1(make(A)) member[side-effect(Parameter-num1, this.num1)]",
			"Function-com_example_A_A_getNum2(make(A)) member[side-effect(Parameter-num2, this.num2)]",
		}, t)
	})
}

func TestJava_Instantiation(t *testing.T) {
	t.Run("Instantiate a non-existent object", func(t *testing.T) {
		code := `
public class Main{
    public static void main(String[] args) {
        File tempFile = new File();
		println(tempFile);
    }
}`
		ssatest.CheckPrintlnValue(code, []string{
			"make(any)",
		}, t)
	})

	t.Run("instantiate an existing object ", func(t *testing.T) {
		code := `
public class File{
}

public class Main{
    public static void main(String[] args) {
        File tempFile = new File();
		println(tempFile);
    }
}`
		ssatest.CheckPrintlnValue(code, []string{
			"make(File)",
		}, t)
	})

}
