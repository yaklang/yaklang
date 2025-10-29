package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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

	t.Run("test static member", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	public 	class A {
			 int a = 0; 
			public void getA(){
				return a;
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

func TestMemberThis(t *testing.T) {
	t.Run("test simple", func(t *testing.T) {
		code := `
package foo.bar;

class A {
	public  int key;

	public void foo() {
		print(this.key.String());
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`key.String() as $target`,
			map[string][]string{
				"target": {"Undefined-this.key.String(ParameterMember-parameter[0].key)"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("set member use this", func(t *testing.T) {
		code := `
package foo.bar;
class A {
	public  int value;
	public void set(int num) {
		this.value = num;
	}
	public void get() {
		return this.value;
	}
	
	public static void main(){
 		A a = new A();
		a.set(12);
		println(a.get());
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> as $result)`,
			map[string][]string{
				"result": {"12"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("read member use this", func(t *testing.T) {
		code := `
package foo.bar;
class A {
	public  int value;
	public void set(int num) {
		value = num;
	}
	public int get() {
		return this.value;
	}
	
	public static void main(){
 		A a = new A();
		a.set(12);
		println(a.get());
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> as $result)`,
			map[string][]string{
				"result": {"12"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("set member  use this", func(t *testing.T) {
		code := `
package foo.bar;
class A {
	public  int value;
	public void set(int num) {
		this.value = num;
	}
	public int get() {
		return value;
	}
	
	public static void main(){
 		A a = new A();
		a.set(12);
		println(a.get());
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> as $result)`,
			map[string][]string{
				"result": {"12"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("set member does not use this", func(t *testing.T) {
		code := `
package foo.bar;
class A {
	public  int value;
	public void set(int num) {
		value = num;
	}
	public int get() {
		return value;
	}
	
	public static void main(){
 		A a = new A();
		a.set(12);
		println(a.get());
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> as $result)`,
			map[string][]string{
				"result": {"12"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	t.Run("static method read and set member ", func(t *testing.T) {
		t.Skip()
		//TODO: wait for oop refractor
		code := `
package foo.bar;
class A {
	public static int value;
	public static void set(int num) {
		value = num;
	}
	public static int get() {
		return value;
	}
	
	public static void main(){
		A.set(12);
		println(A.get());
	}
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* #-> as $result)`,
			map[string][]string{
				"result": {"12"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

}

func Test_Cross_Class_Side_Effect(t *testing.T) {
	t.Skip()
	//TODO:类成员的side-effect要有传递性
	t.Run("aaa", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("a.java", `
package foo.bar;
class A {
	public  int value = 11;

	public void set(int num) {
		this.value = num;
	}
	
	public int get() {
		return this.value;
	}

}
`)
		vf.AddFile("b.java", `
package foo.bar;
class B {
	private A a;

	public B(A a) {
		this.a = a;
	}
	public void set(int num) {
		this.a.set(num);
	}
	public int get() {
		return this.a.get();
	}
	
	public static void main(){
		A a = new A();
		B b = new B(a);
		b.set(22);
		println(b.get());
		b.set(33);
		println(b.get());
	}
}
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			prog.Show()
			ret, err := prog.SyntaxFlowWithError(`println(* #-> as $a);`)
			require.NoError(t, err)
			a := ret.GetValues("a")
			a.Show()
			require.Contains(t, a.String(), "22")
			require.Contains(t, a.String(), "33")
			return nil
		})
	})
}

func Test_Inner_Class(t *testing.T) {
	t.Run("test outerclass.this ", func(t *testing.T) {
		code := `
public class OuterClass {
    private int value = 11;

    class InnerClass {
        private int value = 22;

        public void printValues() {
            println(value);         // 打印内部类的value
            println(OuterClass.this.value); // 打印外部类的value
        }
    }
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* as $result)`, map[string][]string{
				"result": {"ParameterMember-parameter[0].value", "11"},
			}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test outerclass.super", func(t *testing.T) {
		code := `
public class A{
	private int value = 11;
}

public class OuterClass extends A {
	private int value = 22;
	class InnerClass {
		private int value = 33;
		public void printValues() {
			println(value);         // 打印内部类的value
			println(OuterClass.super.value); // 打印外部类父类的value
		}
	}	
}
`
		ssatest.CheckSyntaxFlow(t, code,
			`println(* as $result)`, map[string][]string{
				"result": {"ParameterMember-parameter[0].value", "11"},
			}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestInterfaceDeclarationBlueprint(t *testing.T) {
	code := `
public interface SqliService extends IService<Sqli> {
    int nativeInsert(Sqli user);
    int nativeDelete(Integer id);
    int nativeUpdate(Sqli user);
    Sqli nativeSelect(Integer id);
}
`
	ssatest.CheckSyntaxFlow(t, code, `nativeInsert?{opcode:function}<getCurrentBlueprint> as $result`, map[string][]string{
		"result": {"SqliService"},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
