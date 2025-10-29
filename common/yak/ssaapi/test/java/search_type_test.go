package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSimpleSearchType(t *testing.T) {

	code := `
	class C extends D {}

	class B  extends C {
		void methodB(int i) {
			println(i);
		}
	}

	class A {
		int a; 
		void main() {
			int B = 1;
			B b1  = new B();
			b1.methodB(1);

			B b2  = new B();
			b2.methodB(2);
		}
	}
	`

	// just append Class Instance to cache and database, we can pass this test.
	t.Run("get class instance and variable", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `B as $target`,
			Contain: true,
			Expect: map[string][]string{
				"target": {
					"1",
					"Undefined-B(Undefined-B)",
					"Undefined-B(Undefined-B)",
				},
			},
		})
	})

	t.Run("get extern class instance", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `C as $target`,
			Contain: true,
			Expect: map[string][]string{
				"target": {
					"Undefined-B(Undefined-B)",
					"Undefined-B(Undefined-B)",
				},
			},
		})
	})

	t.Run("get multiple level extern class instance", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `D as $target`,
			Contain: true,
			Expect: map[string][]string{
				"target": {
					"Undefined-B(Undefined-B)",
					"Undefined-B(Undefined-B)",
				},
			},
		})
	})

	t.Run("get class method", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `B.methodB as $target`,
			Contain: false,
			Expect: map[string][]string{
				"target": {
					"Function-B.methodB",
					"Undefined-b1.methodB(valid)",
					"Undefined-b2.methodB(valid)",
				},
			},
		})
	})

	t.Run("get class instance method", func(t *testing.T) {
		test(t, &TestCase{
			Code:    code,
			SF:      `b1.methodB() as $target`,
			Contain: false,
			Expect: map[string][]string{
				"target": {
					"Undefined-b1.methodB(valid)(Undefined-B(Undefined-B),1)",
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
					"Undefined-b1.methodB(valid)(Undefined-B(Undefined-B),1)", "Undefined-b2.methodB(valid)(Undefined-B(Undefined-B),2)",
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
				"target": {"1", "2"},
			},
		})
	})

}

func Test_SearchType_undefine_Class(t *testing.T) {
	code := `
	package com.example.utils;

	class A {
		void main() {
			B b1  = new B();
			b1.methodB(1);
		}
	}	
		`
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(
			t, code,
			`B as $target`,
			map[string][]string{
				"target": {"Undefined-B"},
			}, ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

	// TODO: con't get this method, because B class not found.
	// t.Run("simple method", func(t *testing.T) {
	// 	ssatest.CheckSyntaxFlowContain(
	// 		t, code,
	// 		`B.methodB as $target`,
	// 		map[string][]string{
	// 			"target": {"Undefined-b1.methodB(valid)"},
	// 		}, ssaapi.WithLanguage(ssaconfig.JAVA),
	// 	)
	// })

}
