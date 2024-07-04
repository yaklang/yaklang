package java

import (
	"testing"
)

func Test_Class_Member(t *testing.T) {
	t.Run("simple case 1", func(t *testing.T) {
		test(t, &TestCase{
			Code: `
		class A {
			int a; 
			void main() {
				println(a);
			}
		}
		`,
			SF:      "a --> as $target",
			Contain: false,
			Expect: map[string][]string{
				"target": {"Undefined-println(Undefined-a)"},
			},
		})
	})

	t.Run("simple member field", func(t *testing.T) {
		test(t, &TestCase{
			Code: `
		class A {
			BClass B;
			void main() {
				B.b(1);
				B.b(2);
			}
		}
			`,
			SF: `B.b(* as $target)`,
			Expect: map[string][]string{
				"target": {"1", "2"},
			},
		})
	})

}
