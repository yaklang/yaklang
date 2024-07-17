package java

import (
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
		var aArgs = new String[]{"aaaaaaa"};
        xxeController.yourMethod(aArgs);
    }
}

public class Demo4 {
	
	AnothorController controller = null;

    public String one() throws Exception {
		var flexible = new String[]{"bbbbbb"};
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
			prog.Show()
			results := prog.SyntaxFlow("DocumentBuilderFactory...parse(* #-> as $source) as $sink", sfvm.WithEnableDebug(true))
			results.Show()
			ssatest.CompareResult(t, true, results, map[string][]string{
				"source": {`"Hello Native Method"`, `"aaaaaaa"`},
			})
			return nil
		},
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}
