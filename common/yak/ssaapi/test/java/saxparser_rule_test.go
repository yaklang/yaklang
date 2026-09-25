package java

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestSAXParserRuleMultipleFeatureCalls(t *testing.T) {
	rule, err := os.ReadFile("../../../../syntaxflow/sfbuildin/buildin/java/cwe-611-xxe/java-saxparser-factory-unsafe.sf")
	require.NoError(t, err)
	prog, err := ssaapi.Parse(`import javax.xml.parsers.SAXParserFactory;
class Parser {
void parse(String xml) {
SAXParserFactory factory = SAXParserFactory.newInstance();
factory.setFeature("http://xml.org/sax/features/external-general-entities", false);
factory.setFeature("http://xml.org/sax/features/external-parameter-entities", false);
factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
factory.setFeature("feature-four", false);
factory.setFeature("feature-five", false);
factory.newSAXParser().parse(xml);
}
}`, ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(string(rule))
	require.NoError(t, err)
	require.Empty(t, result.GetValues("vulnCall"))
}
