package syntaxflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const xxeTestCode = `
@RestController(value = "/xxe")
public class XXEController {

    @RequestMapping(value = "/one")
    public String one(@RequestParam(value = "xml_str") String xmlStr) throws Exception {
        DocumentBuilder documentBuilder = DocumentBuilderFactory.newInstance().newDocumentBuilder();
        InputStream stream = new ByteArrayInputStream(xmlStr.getBytes("UTF-8"));
        org.w3c.dom.Document doc = documentBuilder.parse(stream);
        doc.getDocumentElement().normalize();
        return "Hello World";
    }
}

public class XXEFixExample {
    public static void main(String[] args) throws Exception {
        DocumentBuilderFactory dbf = DocumentBuilderFactory.newInstance();

        // Mitigate XXE Attack
        dbf.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
        dbf.setFeature("http://xml.org/sax/features/external-general-entities", false);
        dbf.setFeature("http://xml.org/sax/features/external-parameter-entities", false);
        dbf.setFeature("http://apache.org/xml/features/nonvalidating/load-external-dtd", false);
        dbf.setXIncludeAware(false);
        dbf.setExpandEntityReferences(false);

        DocumentBuilder db = dbf.newDocumentBuilder();

        String xmlData = "<foo>Example</foo>"; // Assume XML data without DOCTYPE
        ByteArrayInputStream xmlStream = new ByteArrayInputStream(xmlData.getBytes());
        Document doc = db.parse(xmlStream);

        System.out.println("Root element :" + doc.getDocumentElement().getNodeName());
    }
}
	`

func Test_Process(t *testing.T) {

	check := func(prog *ssaapi.Program, rule string) {
		process := 0.0
		hasProcess := false
		res, err := prog.SyntaxFlowWithError(rule,
			ssaapi.QueryWithProcessCallback(func(f float64, s string) {
				log.Infof("process callback %f %s", f, s)
				// check has process
				if f > 0 && f < 1 {
					hasProcess = true
				}
				// check is reduce
				if process > f {
					t.Fatal("process is reduce")
					t.FailNow()
				}
				// check is multiple process finish
				if process == 1 {
					t.Fatal("process is multiple finish")
					t.FailNow()
				}
				// update
				if f > process {
					process = f
				}
			}),
		)

		require.Equal(t, process, 1.0)
		require.True(t, hasProcess)
		require.NoError(t, err)
		require.NotNil(t, res)
	}

	t.Run("test normal ", func(t *testing.T) {
		ssatest.Check(t, xxeTestCode, func(prog *ssaapi.Program) error {
			check(prog, `
			DocumentBuilderFactory.newInstance().*Builder().parse(* as $param)
			`)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test loop filter", func(t *testing.T) {
		ssatest.Check(t, xxeTestCode, func(prog *ssaapi.Program) error {
			check(prog, `
	DocumentBuilderFactory.newInstance()?{!((.setFeature) || (.setXIncludeAware) || (.setExpandEntityReferences))} as $entry;
	$entry.*Builder().parse(* #-> as $param);
			`)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})

}

func Test_Context(t *testing.T) {
	t.Run("test context", func(t *testing.T) {
		ssatest.Check(t, xxeTestCode, func(prog *ssaapi.Program) error {
			ctx, cancel := context.WithCancel(context.Background())
			process := 0.0
			_, err := prog.SyntaxFlowWithError(`
			DocumentBuilderFactory.newInstance().*Builder().parse(* as $param)
			`,
				ssaapi.QueryWithContext(ctx),
				ssaapi.QueryWithProcessCallback(func(f float64, s string) {
					log.Infof("process %f : %s", process, s)
					if process < f {
						process = f
					}
					if process >= 0.5 {
						cancel()
					}
				}),
			)
			require.Error(t, err)
			require.Contains(t, err.Error(), "context done")
			require.True(t, process < 1.0)
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
