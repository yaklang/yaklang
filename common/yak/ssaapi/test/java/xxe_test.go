package java

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
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

func TestXXE_WithConditionExpr(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain(`
desc("Description": 'checking setFeature/setXIncludeAware/setExpandEntityReferences in DocumentBuilderFactory.newInstance()')

DocumentBuilderFactory.newInstance()?{any: 'setFeature', 'setXIncludeAware', 'setExpandEntityReferences'} as $entry;
$entry.*Builder().parse() as $result;

check $result then "dangerous xml doc builder" else "safe xml doc builder";

`, sfvm.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("setFeature(*) not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
