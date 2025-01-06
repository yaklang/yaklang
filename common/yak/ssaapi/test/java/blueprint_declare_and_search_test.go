package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Blueprint_name2declare(t *testing.T) {
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
				"classA": {"A_declare"},
				"classC": {"C_declare"},
			}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

	t.Run("search declare class", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A_declare as $classA;
		`, map[string][]string{
			"classA": {"A_declare"},
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

	t.Run("search class relationship", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		A.__children__ as $retA // B  E 
		C.__children__ as $retB // D E CC
		B.__parents__ as $retC // A 
		CC.__parents__  as $retD // C
		D.__parents__ as $retE // C
		E.__parents__ as $retF // A C
		`, map[string][]string{
			"retA": {"B_declare", "E_declare"},
			"retB": {"D_declare", "E_declare", "CC_declare"},
			"retC": {"A_declare"},
			"retD": {"C_declare"},
			"retE": {"C_declare"},
			"retF": {"A_declare", "C_declare"},
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
}

func Test_Blueprint_anonyous_name2declare(t *testing.T) {
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
			res, err := prog.SyntaxFlowWithError(`
			A.__children__ as $classA 
			C.__children__ as $classC
			`)
			require.NoError(t, err)
			res.Show()

			require.True(t, res.GetValues("classA").Len() == 1)
			require.True(t, res.GetValues("classC").Len() == 1)

			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

}

func Test_Blueprint_no_declare(t *testing.T) {
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

			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

	t.Run("relation correct", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		AA.children as $class1 // A, D
		BB.children as $class2 // B, D
		A.parents as $class3 // AA
		B.parents as $class4 // BB
		C.parents as $class5 // CC
		D.parents as $class6 // AA BB
		`, map[string][]string{
			"class1": {"A_declare", "D_declare"},
			"class2": {"B_declare", "D_declare"},
			"class3": {"AA_declare"},
			"class4": {"BB_declare"},
			"class5": {"CC_declare"},
			"class6": {"AA_declare", "BB_declare"},
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
}
