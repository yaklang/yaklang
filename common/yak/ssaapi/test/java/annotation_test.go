package java

import (
	_ "embed"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed sample/annotation.java
var AnnotationBasic string

func TestAnnotation_Negative(t *testing.T) {
	ssatest.Check(t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if result := prog.SyntaxFlowChain("xmlStr --> $ret", sf.WithEnableDebug(true)).Show(); result.Len() <= 0 {
			t.Fatal("xmlStr --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestAnnotation_Positive_Basic1(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-1", t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("Request*._ --> $ret", sf.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("Request*._ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
