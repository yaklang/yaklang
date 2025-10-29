package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Class_Member(t *testing.T) {
	t.Run("simple case 1", func(t *testing.T) {
		test(t, &TestCase{
			Code: `
		class A {
			int a; 
			public static void  main() {
				println(a);
			}
		}
		`,
			SF:      "a --> as $target",
			Contain: true,
			Expect: map[string][]string{
				"target": {"Undefined-println(Undefined-A.a)"},
			},
		})
	})

	t.Run("simple member field", func(t *testing.T) {
		test(t, &TestCase{
			Code: `
		class A {
			BClass B;
			public static void main() {
				B.b(1);
				B.b(2);
			}
		}
			`,
			SF: `B.b(* as $target)`,
			Expect: map[string][]string{
				"target": {"1", "2", "Undefined-A.B"},
			},
		})
	})
	t.Run("simple static member field", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `class A {
			public static BClass B;
			public static void main() {
				B.b(1);
				B.b(2);
			}
		}`, `B.b(* as $target)`, map[string][]string{
			"target": {"1", "2", "Undefined-A.B"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func Test_MethodName_in_Syntaxflow(t *testing.T) {
	t.Run("syntaxflow method name", func(t *testing.T) {
		test(t, &TestCase{
			Code: `
class A {
	int a; 
	public static void F() {
		println(a);
	}
}

public class Main {
    public static void main(String[] args) {

    }
}
		`,
			SF: `
				F as $a
				A_F as $b
			`,
			Contain: true,
			Expect: map[string][]string{
				"a": {"Function-A.F"},
			},
		})
	})
}
