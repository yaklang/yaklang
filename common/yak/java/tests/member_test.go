package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

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
			ssaapi.WithLanguage(ssaapi.JAVA),
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
			ssaapi.WithLanguage(ssaapi.JAVA),
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
			ssaapi.WithLanguage(ssaapi.JAVA),
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
			ssaapi.WithLanguage(ssaapi.JAVA),
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
			ssaapi.WithLanguage(ssaapi.JAVA),
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
			ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	
}
