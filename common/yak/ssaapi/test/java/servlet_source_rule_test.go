package java

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestServletIndirectSourceRuleUsesRegisteredNatives(t *testing.T) {
	rule, err := os.ReadFile("../../../../syntaxflow/sfbuildin/buildin/java/lib/user-input-http-source/java-servlet-params.sf")
	require.NoError(t, err)
	prog, err := ssaapi.Parse(`import javax.servlet.http.HttpServletRequest;
class Handler {
void arbitraryName(HttpServletRequest request) {
println(request.getQueryString());
println(request.getParameterValues("key"));
}
}`, ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	result, err := prog.SyntaxFlowWithError(string(rule))
	require.NoError(t, err)
	require.NotEmpty(t, result.GetValues("indirectQueryString"))
	require.NotEmpty(t, result.GetValues("indirectParamValues"))
}
