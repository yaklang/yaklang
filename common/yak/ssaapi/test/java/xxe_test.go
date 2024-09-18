package java

import (
	"bytes"
	_ "embed"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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

func TestXXE_WithConditionExpr_Basic(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain(`.newInstance()?{.setFeature}`, sfvm.WithEnableDebug(true)).Show().Len() <= 0 {
			t.Fatal("setFeature(*) not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestXXE_WithConditionExpr(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		if ret := prog.SyntaxFlowChain(`
desc("Description": 'checking setFeature/setXIncludeAware/setExpandEntityReferences in DocumentBuilderFactory.newInstance()')

DocumentBuilderFactory.newInstance()?{(.setFeature) || (.setXIncludeAware) || (.setExpandEntityReferences)} as $entry;
$entry.*Builder().parse() as $result;

check $result then "dangerous xml doc builder" else "safe xml doc builder";

`, sfvm.WithEnableDebug(true)).Show(); ret.Len() <= 0 {
			t.Fatal("setFeature(*) not found")
		} else {
			ret.Get(0).ShowWithSourceCode()
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestXXE_WithConditionExprAndSarif(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(`
desc("Description": 'checking setFeature/setXIncludeAware/setExpandEntityReferences in DocumentBuilderFactory.newInstance()')

DocumentBuilderFactory.newInstance()?{!((.setFeature) || (.setXIncludeAware) || (.setExpandEntityReferences))} as $entry;
$entry.*Builder().parse() as $result;

check $result then "dangerous xml doc builder" else "safe xml doc builder";
$result + $entry as $output;
alert $output;

`, sfvm.WithEnableDebug(true))
		if err != nil {
			t.Fatal(err)
		}
		report, err := ssaapi.ConvertSyntaxFlowResultToSarif(result)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		err = report.PrettyWrite(&buf)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(string(buf.String()))
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
