package java

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed sample/xxe.java
var XXE_Code string

func TestXXE(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain(`DocumentBuilderFactory.newInstance().setFeature(*)`).Len() <= 0 {
			t.Fatal("setFeature(*) not found")
		}
		if prog.SyntaxFlowChain(".parse().getDocumentElement()").Show().Len() != 2 {
			t.Fatal("parse().getDocumentElement() not found (not right)")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
