package java

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"strings"
	"testing"
)

func TestNativeCall(t *testing.T) {
	ssatest.Check(t, `

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
}


public class Demo2 {
	@AutoWired
	XXEController xxeController = null;

    public String one() throws Exception {
        xxeController.yourMethod("Hello Native Method");
    }
}


`,
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

			source = prog.SyntaxFlow(`Document*Factory...parse(* #-> *<formalParamToCall> #-> * as $source)`).GetValues("source")

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
