package tests

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[1]",
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
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[side-effect(Parameter-par, this.a)]",
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
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[1]",
		}, t)
	})
	t.Run("test function call", func(t *testing.T) {
		code := `
class A{
	public static void b(){
		return "1";
	}
}
public class main{
	public static void main(String[] args){
		A.a();
	}
}
`
		ssatest.CheckSyntaxFlow(t, code, `A.a() as $call`, map[string][]string{
			"call": {"Undefined-A.a(Undefined-A)"},
		}, ssaapi.WithLanguage(ssaapi.JAVA))
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
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[0]",
			"Undefined-a.getA(valid)(Undefined-A(Undefined-A)) member[side-effect(Parameter-par, this.a)]",
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
			"Undefined-a.getNum(valid)(Undefined-A(Undefined-A)) member[0]",
		}, t)
	})

	t.Run("normal construct", func(t *testing.T) {
		t.Skip()
		code := `
public class A {
	private int num1=0;
	private int num2=0;
	
	// TODO: if this constructor is defined, it will be an error 
	// public A(int num1,int num2) {
	// 	this.num1 = num1;
	// 	this.num2 = num2;
	// }
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
			"Undefined-a.getNum1(valid)(Undefined-A(Undefined-A,1,2)) member[Undefined-a.num1(valid)]",
			"Undefined-a.getNum2(valid)(Undefined-A(Undefined-A,1,2)) member[Undefined-a.num2(valid)]",
		}, t)
	})
}

func TestJava_OOP_Enum(t *testing.T) {
	t.Skip()
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
			"Undefined-a(valid)",
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
			"Undefined-a.getNum1(valid)(Undefined-a(valid)) member[Undefined-a.num1(valid)]",
			"Undefined-a.getNum2(valid)(Undefined-a(valid)) member[Undefined-a.num2(valid)]",
		}, t)
	})

}

func TestJava_OOP_MemberClass(t *testing.T) {
	t.Skip()
	t.Run("test no-static inner class ", func(t *testing.T) {
		code := `
public class Outer {
    public  class Inner{
        int a = 1;
		// TODO: if this constructor is defined, it will be an error
        // public Inner(int par){
        //     this.a=par;
        // }
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
			"Undefined-inner",
		}, t)
	})
}

func TestJava_OOP_Static_Member(t *testing.T) {
	t.Run("test call self static member", func(t *testing.T) {
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
			`, []string{"ParameterMember-parameter[0].a"}, t)
	})

	t.Run("test member variable and static method within a class", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
public class Main {
    int a = 1 ;
    public void main(String[] args) {
            println(a);
        }
 }
			`, []string{"ParameterMember-parameter[0].a"}, t)
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
			"Undefined-a.getNum(valid)(Undefined-A(Undefined-A)) member[0]",
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
			// TODO: this error
			"Undefined-a.getNum1(valid)(Function-A(Undefined-A,1,2)) member[side-effect(Parameter-num1, #16.num1)]", "Undefined-a.getNum2(valid)(Function-A(Undefined-A,1,2)) member[side-effect(Parameter-num2, #16.num2)]",
			// "Undefined-a.getNum1(valid)(Undefined-A(Undefined-A)) member[side-effect(Parameter-num1, this.num1)]",
			// "Undefined-a.getNum2(valid)(Undefined-A(Undefined-A)) member[side-effect(Parameter-num2, this.num2)]",
		}, t)
	})
}

func TestConstruct(t *testing.T) {
	code := `package com.example.demo1;

class Main {
    public int a = 1;

    public Main(int a) {
        this.a = a;
    }
}
class Test{
    public static void main(){
        Main main = new Main(2);
        println(main.a);
    }
}`
	ssatest.CheckPrintlnValue(code, []string{"side-effect(Parameter-a, #13.a)"}, t)
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
		"param": {"2"},
	}, ssaapi.WithLanguage(ssaapi.JAVA))
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
			"Undefined-File(Undefined-File)",
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
			"Undefined-File(Undefined-File)",
		}, t)
	})
	t.Run("test undefind function call", func(t *testing.T) {
		code := `class tes1 {
    public void function(test t) {
        for (int a = 0; ; ) {
            println(t.a());
        }
    }
}`
		ssatest.CheckPrintlnValue(code, []string{"ParameterMember-parameter[1].a(Parameter-t)"}, t)
	})
}

func TestJava_Method(t *testing.T) {
	t.Run("get static method by variable name", func(t *testing.T) {
		code := `
public class ImageUtils{
    public  InputStream getFile(String imagePath){
    }
    public static byte[] readFile(String url){
    }
}
`
		ssatest.CheckSyntaxFlow(t, code, `*readFile as $fun`, map[string][]string{
			"fun": {"Function-ImageUtils.readFile", "Undefined-ImageUtils.readFile(valid)"},
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}
