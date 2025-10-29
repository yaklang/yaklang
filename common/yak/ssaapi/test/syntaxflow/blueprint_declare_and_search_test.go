package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_PHP_Blueprint_name2declare(t *testing.T) {
	code := `
	<?php 
	class A {}
	class B extends A {} // class relation
	interface C {} 
	interface CC extends C{} // interface relation
	class D implements C {} // interface-class relation 
	class E extends A implements C {} // multiple relation
	class F extends FF {}  // FF is no declare class 
	class G implements GG {} // GG  is no declare interface 
	`

	t.Run("search class name to self", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`
			A as $classA
			C as $classC // interface 
			`, map[string][]string{
				"classA": {"A"},
				"classC": {"C"},
			}, ssaapi.WithLanguage(ssaconfig.PHP))
	})

	t.Run("search declare class", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A_declare as $classA;
		`, map[string][]string{
			"classA": {"A"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})

	t.Run("search parent and children relationship", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A.__inherit__ as $retA // B  E 
		C.__implement__ as $retB // D E 
		B.__parents__ as $retC // A 
		CC.__parents__  as $retD // C
		D.__interface__ as $retE // C
		E.__interface__ as $retF //  C
		E.__parents__ as $retG // A
		`, map[string][]string{
			"retA": {"B", "E"},
			"retB": {"D", "E"},
			"retC": {"A"},
			"retD": {"C"},
			"retE": {"C"},
			"retF": {"C"},
			"retG": {"A"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

func Test_PHP_Blueprint_Anonymous_Name2declare(t *testing.T) {
	code := `
	<?php
class A {}
interface C {}

$a = new class extends A {};

$c = new class implements C {};

	`
	t.Run("search class relation-ship", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			res, err := prog.SyntaxFlowWithError(`
		   A.__inherit__ as $retA1

			C.__implement__ as $retC1
			`)
			require.NoError(t, err)
			res.Show()

			require.Equal(t, res.GetValues("retA1").Len(), 1)
			require.Equal(t, res.GetValues("retC1").Len(), 1)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

func Test_PHP_Blueprint_Range(t *testing.T) {
	code := `
<?php
class A extends AA {} 
class B implements BB {} 
interface C extends CC {} 
class D extends AA implements BB {} 
	`
	t.Run("can search and range correct", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(`
			AA as $classAA
			BB as $classBB
			CC as $classCC
			A as $classA
			D as $classD
			`)
			require.NoError(t, err)

			classAAs := res.GetValues("classAA")
			require.Equal(t, classAAs.Len(), 1)
			require.Equal(t, "AA", classAAs[0].GetRange().GetText())

			classBBs := res.GetValues("classBB")
			require.Equal(t, classBBs.Len(), 1)
			require.Equal(t, "BB", classBBs[0].GetRange().GetText())

			classCCs := res.GetValues("classCC")
			require.Equal(t, classCCs.Len(), 1)
			require.Equal(t, "CC", classCCs[0].GetRange().GetText())

			classDs := res.GetValues("classD")
			require.Equal(t, classDs.Len(), 1)
			require.Equal(t, "D", classDs[0].GetRange().GetText())

			classAs := res.GetValues("classA")
			require.Equal(t, classAs.Len(), 1)
			require.Equal(t, "A", classAs[0].GetRange().GetText())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

func Test_PHP_Blueprint_Cycle(t *testing.T) {
	/*
		TODO: PHP当前没有处理 \JsonSerializable这种情况 后续需要特殊处理
	*/
	code := `
<?php

namespace Stripe;

// JsonSerializable only exists in PHP 5.4+. Stub if out if it doesn't exist
if (interface_exists('\JsonSerializable', false)) {
    interface JsonSerializable extends \JsonSerializable
    {
    }
} else {
    // PSR2 wants each interface to have its own file.
    // @codingStandardsIgnoreStart
    interface JsonSerializable
    {
        // @codingStandardsIgnoreEnd
        public function jsonSerialize();
    }
}
interface ArrayAccess{}
class Account extends ApiResource{}
abstract class ApiResource extends StripeObject{}
class StripeObject implements ArrayAccess, JsonSerializable{}
`
	t.Run("no cycle", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		JsonSerializable.__implement__ as $retA
		`, map[string][]string{
			"retA": {"StripeObject"},
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}

func Test_JAVA_Blueprint_name2declare(t *testing.T) {
	code := `
	class A {}
	class B extends A {} // class relation
	interface C {} 
	interface CC extends C{} // interface relation
	class D implements C {} // interface-class relation 
	class E extends A implements C {} // multiple relation
	class F extends FF {}  // FF is no declare class 
	class G implements GG {} // GG  is no declare interface 
	`

	t.Run("search class name to self", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`
			A as $classA
			C as $classC // interface 
			`, map[string][]string{
				"classA": {"A"},
				"classC": {"C"},
			}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("search declare class", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A_declare as $classA;
		`, map[string][]string{
			"classA": {"A"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("search parent and children relationship", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A.__inherit__ as $retA // B  E 
		C.__implement__ as $retB // D E 
		B.__parents__ as $retC // A 
		CC.__parents__  as $retD // C
		D.__interface__ as $retE // C
		E.__interface__ as $retF //  C
		E.__parents__ as $retG // A
		`, map[string][]string{
			"retA": {"B", "E"},
			"retB": {"D", "E"},
			"retC": {"A"},
			"retD": {"C"},
			"retE": {"C"},
			"retF": {"C"},
			"retG": {"A"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func Test_JAVA_Blueprint_Anonymous_Name2declare(t *testing.T) {
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
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			res, err := prog.SyntaxFlowWithError(`
			A.__inherit__ as $retA1

			C.__implement__ as $retC1
			`)
			require.NoError(t, err)
			res.Show()

			require.Equal(t, res.GetValues("retA1").Len(), 1)
			require.Equal(t, res.GetValues("retC1").Len(), 1)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func Test_JAVA_Blueprint_Range(t *testing.T) {
	code := `
	// in class declaration
	class A extends AA {}    // AA is no declare class 
	class B implements BB {} // BB  is no declare interface 
	// in interface declaration
	interface C extends CC {} // CC is no declare interface 
	class D extends AA implements BB {}
	`
	t.Run("can search and range correct", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			res, err := prog.SyntaxFlowWithError(`
			AA as $classAA 
			BB as $classBB
			CC as $classCC
			A as $classA
			D as $classD
			`)
			require.NoError(t, err)

			classAAs := res.GetValues("classAA")
			require.Equal(t, classAAs.Len(), 1)
			require.Equal(t, classAAs[0].GetRange().GetText(), "AA")

			classBBs := res.GetValues("classBB")
			require.Equal(t, classBBs.Len(), 1)
			require.Equal(t, classBBs[0].GetRange().GetText(), "BB")

			classCCs := res.GetValues("classCC")
			require.Equal(t, classCCs.Len(), 1)
			require.Equal(t, classCCs[0].GetRange().GetText(), "CC")

			classDs := res.GetValues("classD")
			require.Equal(t, classDs.Len(), 1)
			require.Equal(t, classDs[0].GetRange().GetText(), "D")

			classAs := res.GetValues("classA")
			require.Equal(t, classAs.Len(), 1)
			require.Equal(t, classAs[0].GetRange().GetText(), "A")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
