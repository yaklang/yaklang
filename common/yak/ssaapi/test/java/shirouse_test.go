package java

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample/shirouse.java
var Code_ShiroUse string

func TestShiroUseJava(t *testing.T) {
	ssatest.Check(t, Code_ShiroUse, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain(`.getCred*()`).Len() <= 0 {
			t.Fatal("getCred*() not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
