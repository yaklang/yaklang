package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// TestFastPath_LogForgingIncludeHits verifies the simple `* & $source`
// include fast path is actually exercised by the real Java log-forging rule.
func TestFastPath_LogForgingIncludeHits(t *testing.T) {
	// The java-servlet-param / java-spring-mvc-param / java-log-record lib
	// rules used by the include below must be synced into the database first,
	// otherwise the <include(...)> sub-rules resolve to nothing and sink is
	// empty.
	yakit.InitialDatabase()
	require.NoError(t, sfbuildin.SyncEmbedRule())

	fs := filesys.NewVirtualFs()
	fs.AddFile("demo/A.java", `package demo;
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
`)

	beforeHit, _ := ssaapi.FastPathMatchStats()
	ssatest.CheckWithFS(fs, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		rule := "<include(\"java-servlet-param\")> as $source;\n" +
			"<include(\"java-spring-mvc-param\")> as $source;\n" +
			"<include(\"java-log-record\")> as $log;\n" +
			"$log#{include:`* & $source`}-> as $dest;\n" +
			"$dest<getPredecessors> as $sink;\n"
		res, err := prog.SyntaxFlowWithError(rule)
		require.NoError(t, err)
		require.NotEmpty(t, res.GetValues("sink"), "log forging should find a sink")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

	afterHit, _ := ssaapi.FastPathMatchStats()
	require.Greater(t, afterHit-beforeHit, int64(0),
		"the real Java log-forging include must hit the fast path")
}
