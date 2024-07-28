package java

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"

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
		if prog.SyntaxFlowChain("Request*.__ref__ --> as $ret", sf.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("Request*.__ref__ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

//go:embed sample/formal_param_annotation.java
var FormalParamAnnotationBasic string

func TestAnnotation_Postive_FormalParam(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-2", t, FormalParamAnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("*Param.__ref__ --> as $ret", sf.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*Param.__ref__ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestAnnotation_Postive_FormalParam_2(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-3", t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("*Param.__ref__ --> as $ret", sf.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*Param.__ref__ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestAnnotation_Positive_Basic2(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-TestAnnotation_Positive_Basic2", t, `
package com.vuln.controller;

public class DemoABCEntryClass {
    @RequestMapping(value = "/one")
    public String methodEntry(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        return "Hello World" + xmlStr;
    }
}
`, func(prog *ssaapi.Program) error {
		t.Log("checking xmlStr?{opcode: param}.annotation.*Param.value as $ref")
		assert.Greater(t, prog.SyntaxFlowChain("xmlStr?{opcode: param}.annotation.*Param.value as $ref", sf.WithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking .annotation.*Param.value as $ref")
		assert.Greater(t, prog.SyntaxFlowChain(".annotation.*Param.value as $ref", sf.WithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking *Param.value as $ref")
		assert.Greater(t, prog.SyntaxFlowChain("*Param.value as $ref", sf.WithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking *Param.__ref__?{const} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain("*Param.__ref__?{opcode: const} as $ref", sf.WithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking *Param.__ref__?{opcode: param} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain("*Param.__ref__?{opcode: param} as $ref", sf.WithEnableDebug(false)).Show().Len(), 1)

		t.Log("checking *Param.__ref__?{opcode: param && .annotation} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain("*Mapping.__ref__(*?{opcode: param && .annotation} as $ref )", sf.WithEnableDebug(false)).Show().Len(), 1)

		t.Log("checking *Param.__ref__?{opcode: param && !have: this} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain("*Mapping.__ref__(*?{opcode: param && !have: this} as $ref )", sf.WithEnableDebug(false)).Show().Len(), 1)

		t.Log("checking *Param.__ref__?{have: this} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain("*Mapping.__ref__(*?{have: this} as $ref )", sf.WithEnableDebug(false)).Show().Len(), 1)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestAnnotation_GetTopDef(t *testing.T) {
	code := `
public class XXEController {
    public void yourMethod(@RequestParam(value = "xml_str") String xmlStr) {
        println(xmlStr.a);
    }
}

public class Demo3 {
	XXEController xxeController ;
    public String one() {
		var aArgs = new String[]{"aaaaaaa"};
        xxeController.yourMethod(aArgs);
    }
}
	`
	ssatest.CheckSyntaxFlowContain(t, code, `
	println(* #-> as $target)
	`, map[string][]string{
		"target": {`"aaaaaaa"`},
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
