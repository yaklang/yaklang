package java

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestDataflowExcludePruneKeepsEscapeSemantics exercises the monotone
// `*<getCallee> & $filter` branch cut: the escaped sink must still be excluded,
// while the raw sink keeps its tainted top definition.
func TestDataflowExcludePruneKeepsEscapeSemantics(t *testing.T) {
	libraries := make(map[string]*schema.SyntaxFlowRule)
	for name, path := range map[string]string{
		"java-servlet-param": "lib/user-input-http-source/java-servlet-params.sf",
		"java-escape-method": "lib/filter/common/java-escape-method.sf",
	} {
		raw, err := os.ReadFile("../../../../syntaxflow/sfbuildin/buildin/java/" + path)
		require.NoError(t, err)
		library, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		require.NoError(t, err)
		libraries[name] = library
	}
	ctx := ssaapi.WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), libraries)

	const source = `package demo;
import java.io.PrintWriter;
import javax.servlet.http.*;
public class Prune extends HttpServlet {
    public void raw(HttpServletRequest req, HttpServletResponse res) throws Exception {
        String name = req.getParameter("name");
        PrintWriter w = res.getWriter();
        w.println(name);
    }
    public void escaped(HttpServletRequest req, HttpServletResponse res) throws Exception {
        String name = req.getParameter("name");
        String safe = escapeHtml(name);
        PrintWriter w = res.getWriter();
        w.println(safe);
    }
    static String escapeHtml(String s) { return s; }
}
`
	rule := `<include('java-servlet-param')> as $source;
<include('java-escape-method')> as $filter;
*.getWriter()?{<getCallee><typeName>?{have:'HttpServletResponse'}} as $out;
$out.println(,* as $sink);
$sink#{include:` + "`* & $source`, exclude:`*<getCallee> & $filter`" + `}-> as $mid;
$sink#{include:` + "`* & $source`" + `}-> as $all;
`

	prog, err := ssaapi.Parse(source, ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithContext(ctx), ssaapi.QueryWithMemory())
	require.NoError(t, err)

	mid := res.GetValues("mid")
	all := res.GetValues("all")
	require.NotEmpty(t, mid, "the unescaped println must keep its tainted top definition")
	require.NotEmpty(t, all)
	require.Greater(t, len(all), len(mid), "the escaped println must be pruned from the exclude-filtered result")

	for _, v := range mid {
		fn := v.GetFunction()
		if fn == nil {
			continue
		}
		require.NotContains(t, strings.ToLower(fn.GetName()), "escaped",
			"a top definition reached through the escape helper must not survive the exclude filter")
	}
}
