package java

import "testing"

func TestSimpleSearchType(t *testing.T) {

	code := `

	class B {
		void methodB(int i) {
			println(i);
		}
	}

	class A {
		int a; 
		void main() {
			B b1  = new B();
			b1.methodB(1);

			B b2  = new B();
			b2.methodB(2);
		}
	}
	`

	// just append Class Instance to cache and database, we can pass this test.
	t.Run("get class instance", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `B as $target`,
			Contain: false,
			Expect: map[string][]string{
				"target": {
					/* TODO: type error when code unmarshal
					from source, this instruction will be `make(B)`,
					from database, type will be any, so this will be `make(any)`,
					*/
					"make(B)",
					"make(B)",
				},
			},
		})
	})

	/* TODO: get class method/member
	`b1.methodB` will be compiled to function, so con't get this value use member-object relationship.

	in java `b1.methodB()` will be compiled to `B_methodB(b1)`, and this function used in
	getTop/getBottom, but we con't get this value use `B.methodB` in syntaxFlow, this not
	member-object relationship.

	so we should modify this code, `b1.methodB()` to `undefine-b1.methodB(b1)`, and set method type,
	and use function-type get return instruction, when handler this call-instruction in getTop/getBottom.
	*/
	t.Run("get class method", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `B.methodB as $target`,
			Contain: false,
			Expect: map[string][]string{
				"target": {
					"Undefined-b1.methodB(valid)",
					"Undefined-b2.methodB(valid)",
				},
			},
		})
	})

	t.Run("get class method call", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `B.methodB() as $target`,
			Contain: false,
			Expect: map[string][]string{
				"target": {
					"Undefined-b1.methodB(valid)(make(B),1)",
					"Undefined-b2.methodB(valid)(make(B),2)",
				},
			},
		})
	})

	t.Run("method function should has called", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `println(* #-> * as $target)`,
			Contain: false,
			Expect: map[string][]string{
				"target": {"1", "2", "Parameter-i"},
			},
		})
	})

}
