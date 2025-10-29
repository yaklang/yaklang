package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSF_NativeCall_Blueprint(t *testing.T) {
	code := `
	class A {}
	class B extends A {} // class relation
	interface C {} 
	interface CC extends C{} // interface relation
	class D implements C {} // interface-class relation 
	class E extends A implements C {} // multiple relation
	class F extends FF {}  // FF is no declare class 
	class G implements GG {} // GG  is no declare interface 
	class H extends B {} 
	
	`
	t.Run("search parent and children relationship", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		B<getParentsBlueprint> as $retA // A 
		CC<getParentsBlueprint>  as $retB // C
		D<getInterfaceBlueprint> as $retC// C
		E<getInterfaceBlueprint> as $retD //  C
		E<getParentsBlueprint> as $retE // A

		H<getRootParentBlueprint> as $root1
		`, map[string][]string{
			"retA":  {"A"},
			"retB":  {"C"},
			"retC":  {"C"},
			"retD":  {"C"},
			"retE":  {"A"},
			"root1": {"A"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
