package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_OOP_className(t *testing.T) {
	code := `
	<?php 
	class A {}
	class B extends A {} // class relation

	interface C {} 
	interface CC extends C{} // interface relation

	class D implements C {} // interface-class relation 

	class E extends A implements C {}
	`

	t.Run("search class by self name", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`
			A as $classA
			C as $classC // interface 
			`, map[string][]string{
				"classA": {"A-declare"},
				"classC": {"C-declare"},
			}, ssaapi.WithLanguage(ssaapi.PHP),
		)
	})

	t.Run("search class relationship", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A.children as $classA // B  E 
		C.children as $classC // D E

		B.parents as $classB // A 
		CC.parents as $classCC // C
		D.parents as $classD // C
		E.parents as $classE // A C
		`, map[string][]string{
			"classA":  {"B-declare", "E-declare"},
			"classC":  {"D-declare", "E-declare", "CC-declare"},
			"classB":  {"A-declare"},
			"classD":  {"C-declare"},
			"classCC": {"C-declare"},
			"classE":  {"A-declare", "C-declare"},
		}, ssaapi.WithLanguage(ssaapi.PHP),
		)
	})
}
