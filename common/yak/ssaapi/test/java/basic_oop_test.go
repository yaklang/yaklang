package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_OOP_className(t *testing.T) {
	code := `

	class A {}
	class B extends A {}

	interface C {} 

	class D implements C {}

	class E extends A implements C {}
	`

	t.Run("search class name to self", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`
			A as $classA
			C as $classC // interface 
			`, map[string][]string{
				"classA": {"A-declare"},
				"classC": {"C-declare"},
			}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

	t.Run("search class relation-ship", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A.children as $classA // B  E 
		C.children as $classC // D E

		B.parents as $classB // A 
		D.parents as $classD // C
		E.parents as $classE // A C
		`, map[string][]string{
			"classA": {"B-declare", "E-declare"},
			"classC": {"D-declare", "E-declare"},
			"classB": {"A-declare"},
			"classD": {"C-declare"},
			"classE": {"A-declare", "C-declare"},
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
}

func Test_OOP_className_anonymous_class(t *testing.T) {
	code := `
	class A {} 
	interface C {} 
	class O {
		static void main(){
			A a = new A() {}; // anonymous class extends A
			C c = new C() {}; // anonymous class implements C 
		}
	}
	`

	t.Run("search class relation-ship", func(t *testing.T) {

		ssatest.CheckSyntaxFlow(t, code, `
		A.children as $classA // anonymous class
		C.children as $classC // anonymous class
		`, map[string][]string{})
	})

	// new Book("Design Patterns") {
	// 	@Override
	// 	public String description() {
	// 		return "Famous GoF book.";
	// 	}
	// }
}
