package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Blueprint_search_instance(t *testing.T) {
	code := `
	class A{
		A get() {}
	} 

	class Main {
		static void main(String[] args) {
			A a = new A();
		}
		void ff(A a) {
			b = a.get(); 
		}
	}
	`

	t.Run("search instance", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, code,
			`A as $classA`, map[string][]string{
				"classA": {
					"A_declare",
					"Undefined-A-constructor(Undefined-A)", // new A()
					"Parameter-a",                          // ff(A a)

				},
			}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

}
