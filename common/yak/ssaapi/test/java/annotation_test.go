package java

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample/annotation.java
var AnnotationBasic string

func TestAnnotation_Negative(t *testing.T) {
	ssatest.Check(t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if result := prog.SyntaxFlowChain("xmlStr --> as $ret", ssaapi.QueryWithEnableDebug(true)).Show(); result.Len() <= 0 {
			t.Fatal("xmlStr --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestAnnotation_Positive_Basic1(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-1", t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("Request*.__ref__ --> as $ret", ssaapi.QueryWithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("Request*.__ref__ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

//go:embed sample/formal_param_annotation.java
var FormalParamAnnotationBasic string

func TestAnnotation_Postive_FormalParam(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-2", t, FormalParamAnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("*Param.__ref__ --> as $ret", ssaapi.QueryWithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*Param.__ref__ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestAnnotation_Postive_FormalParam_2(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-3", t, AnnotationBasic, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("*Param.__ref__ --> as $ret", ssaapi.QueryWithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("*Param.__ref__ --> $ret not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
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
		assert.Greater(t, prog.SyntaxFlowChain("xmlStr?{opcode: param}.annotation.*Param.value as $ref", ssaapi.QueryWithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking .annotation.*Param.value as $ref")
		assert.Greater(t, prog.SyntaxFlowChain(".annotation.*Param.value as $ref", ssaapi.QueryWithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking *Param.value as $ref")
		assert.Greater(t, prog.SyntaxFlowChain("*Param.value as $ref", ssaapi.QueryWithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking *Param.__ref__?{const} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain("*Param.__ref__?{opcode: const} as $ref", ssaapi.QueryWithEnableDebug(false)).Show().Len(), 0)

		t.Log("checking *Param.__ref__?{opcode: param} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain("*Param.__ref__?{opcode: param} as $ref", ssaapi.QueryWithEnableDebug(false)).Show().Len(), 1)

		t.Log("checking *Param.__ref__?{opcode: param && .annotation} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain(`
*Mapping.__ref__<getFormalParams>?{opcode: param && !have: this} as $ref
`, ssaapi.QueryWithEnableDebug(false)).Show().Len(), 1)

		t.Log("checking *Param.__ref__?{opcode: param && !have: this} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain(`
*Mapping.__ref__<getFormalParams>?{opcode: param && !have: this} as $ref
`, ssaapi.QueryWithEnableDebug(false)).Show().Len(), 1)

		t.Log("checking *Param.__ref__?{have: this} as $ref")
		assert.Equal(t, prog.SyntaxFlowChain(`
*Mapping.__ref__<getFormalParams>?{have: this} as $ref
`, ssaapi.QueryWithEnableDebug(false)).Show().Len(), 1)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
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
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestInterfaceAnnotation(t *testing.T) {
	ssatest.CheckJava(t, `

@lokjasdgjlkassdfjlkjloasdfijloa("hk;aabbccddeeff;asdljk")
public class HomeDaoClassABC {
    List<PmsBrand> aaab(@Param("offset") Integer offset,@Param("limit") Integer limit) {
		return null;
	};
}

@ClassAnnotationTest
public class HomeDaoClassABC {
    List<PmsBrand> abasdfasdfasdfbar(@Param("offset") Integer offset,@Param("limit") Integer limit) {
		return null;
	};
}

@TestInterfaceAnno
public interface HomeDao {
    List<PmsBrand> getRecommendBrandList(@Param("offset") Integer offset,@Param("limit") Integer limit);
}

@TestInterfaceAnno2("bb")
public interface HomeDao3 {
    List<PmsBrand> getRecommendBrandList(String abc);
}

`, func(prog *ssaapi.Program) error {
		prog.Show()
		var results ssaapi.Values

		results = prog.SyntaxFlowChain(`.annotation.*?{.value<regexp('aabbccddeeff')>}.__ref__.*ab<getFormalParams>*?{any: limit,offset} as $params`).Show()
		assert.GreaterOrEqual(t, results.Len(), 2)

		results = prog.SyntaxFlowChain(`.annotation.ClassAnnotationTest.__ref__.*bar<getFormalParams>?{any: limit,offset} as $params`).Show()
		assert.GreaterOrEqual(t, results.Len(), 2)

		results = prog.SyntaxFlowChain(`.annotation.*Anno.__ref__.*List<getFormalParams>?{any: limit,offset} as $params`).Show()
		assert.GreaterOrEqual(t, results.Len(), 2)

		results = prog.SyntaxFlowChain(`.annotation.*Anno2.value<regexp('bb')><show>`)
		assert.GreaterOrEqual(t, results.Len(), 1)
		return nil
	})
}

func TestAnnotation_MutliAnnotation(t *testing.T) {
	ssatest.CheckWithName("annotation-basic-TestAnnotation_Mutli_Annotation", t, `
package com.vuln.controller;

@Controller("aa")
@ResponseBody("bb")
public class DemoABCEntryClass {
    @PostMapping("/")
    @ResponseStatus("200")
    public String methodEntry(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        return "Hello World" + xmlStr;
    }
}
`, func(prog *ssaapi.Program) error {
		prog.Show()
		assert.Equal(t, prog.SyntaxFlowChain("Controller.__ref__ as $ref ", ssaapi.QueryWithEnableDebug(false)).Show(false).Len(), 1)
		assert.Equal(t, prog.SyntaxFlowChain("ResponseBody.__ref__ as $ref ", ssaapi.QueryWithEnableDebug(false)).Show(false).Len(), 1)
		assert.Equal(t, prog.SyntaxFlowChain(".annotation.Controller.value?{have:'aa'} as $ref ", ssaapi.QueryWithEnableDebug(false)).Show(false).Len(), 1)
		assert.Equal(t, prog.SyntaxFlowChain(".annotation.ResponseBody.value?{have:'bb'} as $ref ", ssaapi.QueryWithEnableDebug(false)).Show(false).Len(), 1)

		assert.Equal(t, prog.SyntaxFlowChain("*Mapping.__ref__ as $ref", ssaapi.QueryWithEnableDebug(false)).Show(false).Len(), 1)
		assert.Equal(t, prog.SyntaxFlowChain("ResponseStatus.__ref__ as $ref", ssaapi.QueryWithEnableDebug(false)).Show(false).Len(), 1)
		assert.Equal(t, prog.SyntaxFlowChain(".annotation.*Mapping.value?{have:'/'} as $ref", ssaapi.QueryWithEnableDebug(false)).Show(false).Len(), 1)
		assert.Equal(t, prog.SyntaxFlowChain(".annotation.ResponseStatus.value?{have:'200'} as $ref ", ssaapi.QueryWithEnableDebug(false)).Show().Len(), 1)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
