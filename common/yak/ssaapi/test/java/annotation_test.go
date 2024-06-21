package java

import (
	_ "embed"
	"testing"

	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample/annotation.java
var AnnotationBasic string

func TestAnnotation_Negative(t *testing.T) {
	ssatest.Check(t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if result := prog.SyntaxFlowChain("xmlStr --> as $ret", sf.WithEnableDebug(true)).Show(); result.Len() <= 0 {
			t.Fatal("xmlStr --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestAnnotation_Positive_Basic1(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-1", t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("Request*._ --> as $ret", sf.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("Request*._ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

//go:embed sample/formal_param_annotation.java
var FormalParamAnnotationBasic string

func TestAnnotation_Postive_FormalParam(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-2", t, FormalParamAnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("*Param._ --> as $ret", sf.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*Param._ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestAnnotation_Postive_FormalParam_2(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-3", t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("*Param._ --> as $ret", sf.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*Param._ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
