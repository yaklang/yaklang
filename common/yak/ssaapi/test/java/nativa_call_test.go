package java

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"strings"
	"testing"
)

const NativeCallTest = `

@RestController(value = "/xxe")
public class XXEController {

    @RequestMapping(value = "/one")
    public String yourMethod(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        DocumentBuilder documentBuilder = DocumentBuilderFactory.newInstance().newDocumentBuilder();
        InputStream stream = new ByteArrayInputStream(xmlStr.getBytes("UTF-8"));
        org.w3c.dom.Document doc = documentBuilder.parse(stream);
        doc.getDocumentElement().normalize();
        return "Hello World";
    }

    public String HHHHH(@RequestParam(value = "xxx") String xxxFooBar) throws Exception {
        return "Hello getReturns";
    }
}


public class Demo2 {
	@AutoWired
	XXEController xxeController = null;

    public String one() throws Exception {
        xxeController.yourMethod("Hello Native Method");
    }
}

public class Demo3 {
	@AutoWired
	XXEController xxeController = null;

    public String one() throws Exception {
		var aArgs = new String[]{"AARGS"};
        xxeController.yourMethod(aArgs);
    }
}

public class Demo4 {
	
	AnothorController controller = null;

    public String one() throws Exception {
		var flexible = new String[]{"AARGS"};
        controller.yourMethod(flexible);
    }
}

`

func TestNativeCall_GetObject(t *testing.T) {
	ssatest.Check(t, `a = {"b": 111, "c": 222, "e": 333}`,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
.b<getObject>.c as $sink;
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			if !strings.Contains(sink.String(), "222") {
				t.Fatal("sink[0].String() != 222")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.Yak),
	)
}

func TestNativeCall_GetReturns(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
HHHHH <getReturns> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			if !strings.Contains(sink.String(), "Hello getReturns") {
				t.Fatal("sink[0].String() != Hello getReturns")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetFormalParams(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
HHHHH <getFormalParams> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink").Show()
			if sink.Len() != 2 {
				t.Fatal("sink.Len() != 2")
			}

			if !utils.MatchAllOfSubString(sink.String(), "xxxFooBar", "this") {
				t.Fatal("sink[0].String() !contains xxxFooBar / this")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_SearchCall(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
flexible <getCall> <searchFunc> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			if sink.Len() <= 1 {
				t.Fatal("sink.Len() <= 1")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetCall(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
aArgs <getCall> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetCall_Then_GetFunc(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
aArgs <getCall> <getFunc> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			spew.Dump(sink[0].GetName())
			if !strings.HasSuffix(sink[0].GetName(), "yourMethod") {
				t.Fatal("sink[0].GetName() != yourMethod")
			}

			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_GetFunc(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow(`
aArgs <getFunc> as $sink; 
`, sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			spew.Dump(sink[0].GetName())
			if !strings.HasSuffix(sink[0].GetName(), "yourMethod") {
				t.Fatal("sink[0].GetName() != yourMethod")
			}

			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func TestNativeCall_SearchFormalParams(t *testing.T) {
	ssatest.Check(t, NativeCallTest,
		func(prog *ssaapi.Program) error {
			results := prog.SyntaxFlow("DocumentBuilderFactory...parse(* #-> *?{opcode: param} as $source) as $sink", sfvm.WithEnableDebug(true))
			sink := results.GetValues("sink")
			if sink.Len() != 1 {
				t.Fatal("sink.Len() != 1")
			}
			source := results.GetValues("source")
			if source.Len() != 1 {
				t.Fatal("source.Len() != 1")
			}

			source = prog.SyntaxFlow(`Document*Factory...parse(* #-> ?{opcode: param}<searchFunc> #-> * as $source)`).GetValues("source")

			check := false
			if source.Show().Recursive(func(operator sfvm.ValueOperator) error {
				rfaw := operator.String()
				spew.Dump(rfaw)
				if strings.Contains(rfaw, "Hello World") {
					check = true
				}
				return nil
			}) != nil {
				t.Fatal("source.Show().Len() != 1")
			}
			if !check {
				t.Fatal("check FormalParamToCall failed")
			}
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}
