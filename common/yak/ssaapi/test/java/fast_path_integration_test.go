package java

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestFastPath_LogForgingIncludeHits verifies the simple `* & $source`
// include fast path is actually exercised by the real Java log-forging rule.
func TestFastPath_LogForgingIncludeHits(t *testing.T) {
	// Load the complete real library sources used by this query, without
	// syncing 1196 unrelated rules or relying on a previously populated DB.
	libraries := make(map[string]*schema.SyntaxFlowRule)
	for name, path := range map[string]string{
		"java-servlet-param":    "user-input-http-source/java-servlet-params.sf",
		"java-spring-mvc-param": "user-input-http-source/java-spring-mvc-params.sf",
		"java-log-record":       "log/java-log-record.sf",
	} {
		raw, err := os.ReadFile("../../../../syntaxflow/sfbuildin/buildin/java/lib/" + path)
		require.NoError(t, err)
		library, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		require.NoError(t, err)
		require.True(t, library.AllowIncluded)
		require.Equal(t, name, library.IncludedName)
		libraries[name] = library
	}
	ctx := ssaapi.WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), libraries)

	const source = `package demo;
import javax.servlet.http.*;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
public class A extends HttpServlet {
    private static final Logger log = LoggerFactory.getLogger(A.class);
    public void doGet(HttpServletRequest req, HttpServletResponse res) {
        String v = req.getParameter("x");
        log.info("value=" + v);
    }
}
`

	for _, tc := range []struct {
		name, source string
		wantSink     bool
	}{
		{"request reaches log", source, true},
		{"constant log is safe", strings.Replace(source, `log.info("value=" + v)`, `log.info("constant")`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := filesys.NewVirtualFs()
			fs.AddFile("demo/A.java", tc.source)
			beforeHit, _ := ssaapi.FastPathMatchStats()
			ssatest.CheckWithFS(fs, t, func(progs ssaapi.Programs) error {
				prog := progs[0]
				rule := "<include(\"java-servlet-param\")> as $source;\n" +
					"<include(\"java-spring-mvc-param\")> as $source;\n" +
					"<include(\"java-log-record\")> as $log;\n" +
					"$log#{include:`* & $source`}-> as $dest;\n" +
					"$dest<getPredecessors> as $sink;\n"
				res, err := prog.SyntaxFlowWithError(rule, ssaapi.QueryWithContext(ctx), ssaapi.QueryWithMemory())
				require.NoError(t, err)
				if tc.wantSink {
					require.NotEmpty(t, res.GetValues("sink"), "log forging should find a sink")
				} else {
					require.Empty(t, res.GetValues("sink"), "constant logging must not be a finding")
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.JAVA))

			afterHit, _ := ssaapi.FastPathMatchStats()
			if tc.wantSink {
				require.Greater(t, afterHit-beforeHit, int64(0),
					"the real Java log-forging include must hit the fast path")
			}
		})
	}
}
