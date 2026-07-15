package syntaxflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// These tests document SSA taint-propagation gaps observed while triaging
// irify-benchmark OWASP Benchmark LDAP injection (CWE-90) false negatives.
//
// Context: the LDAP rule fires correctly on direct getParameter ->
// DirContext.search flows when verified via ssa-query (it produces 26+
// matches on the LDAP testcode program). However, code-scan in built-in rule
// mode produces 0 risks for the LDAP rule — the dataflow include filter
// does not connect source to sink in the batch scan pipeline. This is a
// separate code-scan pipeline issue, not a rule issue.
//
// Of the 27 true-positive cases, 21 use an interprocedural return-value
// pattern (doSomething) and 6 use direct/collection-aliasing patterns.
// All 32 false-positive cases are correctly not alerted (no FP).
//
// Classification (verified against OWASP Benchmark v1.2 expected results):
//   - DirectPropagation        : true vuln, must fire.    (control)
//   - InterproceduralReturn     : true vuln, currently does NOT fire. SSA gap.
//   - HashMapAliasing           : true vuln, currently does NOT fire. SSA gap.
//   - URLDecodePropagation      : true vuln, currently does NOT fire. SSA gap.

// ldapPropagationRule mirrors the real java-ldap-injection.sf rule but
// stripped to the minimum needed to exercise propagation.
var ldapPropagationRule = strings.NewReplacer(
	"\t", " ",
).Replace(strings.TrimSpace(`
*?{opcode:param}?{<typeName>?{have:'HttpServletRequest'}} as $req;
$req.getParameter() as $source;
.search?{<typeName>?{have:'javax.naming'}}(* as $sink);
check $sink;
$sink#{include: "<self> & $source"}-> as $tainted;
alert $tainted for { title: "ldap propagation test" }
`))

// runLDAPPropagationCase compiles src as Java, runs the propagation rule,
// and asserts that the $tainted alert fires.
func runLDAPPropagationCase(t *testing.T, src string) {
	t.Helper()
	prog, err := ssaapi.Parse(strings.TrimSpace(src),
		ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(ldapPropagationRule)
	require.NoError(t, err)
	require.NotNil(t, res)
	tainted := res.GetValues("tainted")
	if len(tainted) == 0 {
		t.Fatalf("taint did not propagate: $tainted is empty (source/sink both matched, propagation gap)")
	}
}

// TestLDAP_DirectPropagation is the control case: getParameter result flows
// directly to DirContext.search. Must pass — it proves the rule + source.
func TestLDAP_DirectPropagation(t *testing.T) {
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.naming.directory.DirContext;
import javax.naming.directory.SearchControls;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        DirContext ctx = null;
        SearchControls sc = new SearchControls();
        String filter = "(uid=" + param + ")";
        ctx.search("ou=users", filter, sc);
    }
}
`
	runLDAPPropagationCase(t, src)
}

// TestLDAP_InterproceduralReturnPropagation documents that taint does not
// propagate through a method that receives a tainted parameter and returns it.
//
// Reproduces OWASP Benchmark LDAP cases (21 of 27 FN cases):
//
//	String param = request.getParameter("...");
//	String bar = new Test().doSomething(request, param);
//	String filter = "(uid=" + bar + ")";
//	ctx.search(base, filter, sc);
//
// The doSomething method simply assigns param to bar and returns it.
// Taint should flow: param -> doSomething(param) -> return value -> bar ->
// filter -> ctx.search. Currently it does NOT.
func TestLDAP_InterproceduralReturnPropagation(t *testing.T) {
	skipXPathPropagation(t, "interprocedural return value: taint passed to a method parameter and returned is not tracked back to the caller's variable")
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.naming.directory.DirContext;
import javax.naming.directory.SearchControls;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        String bar = new Test().doSomething(req, param);
        DirContext ctx = null;
        SearchControls sc = new SearchControls();
        String filter = "(uid=" + bar + ")";
        ctx.search("ou=users", filter, sc);
    }
    class Test {
        String doSomething(HttpServletRequest req, String param) {
            String bar;
            int num = 196;
            if ((500 / 42) + num > 200) bar = param;
            else bar = "safe";
            return bar;
        }
    }
}
`
	runLDAPPropagationCase(t, src)
}

// TestLDAP_HashMapAliasingPropagation documents that taint does not propagate
// through a HashMap put-then-get pattern.
//
// Reproduces OWASP Benchmark LDAP cases (e.g. BenchmarkTest00695):
//
//	String param = request.getParameterValues("...")[0];
//	String bar = "safe!";
//	HashMap<String, Object> map = new HashMap<>();
//	map.put("keyB", param);
//	bar = (String) map.get("keyB");
//	String filter = "(uid=" + bar + ")";
//	ctx.search(base, filter, sc);
//
// Taint should flow: param -> map.put -> map.get -> bar -> filter -> search.
// Currently it does NOT — the SSA pipeline does not track taint through
// HashMap put-get aliasing.
func TestLDAP_HashMapAliasingPropagation(t *testing.T) {
	skipXPathPropagation(t, "HashMap aliasing: taint through map.put(key, param) -> map.get(key) is not tracked")
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.naming.directory.DirContext;
import javax.naming.directory.SearchControls;
import java.util.HashMap;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        String bar = "safe!";
        HashMap<String, Object> map = new HashMap<String, Object>();
        map.put("keyA", "a-Value");
        map.put("keyB", param);
        map.put("keyC", "c-Value");
        bar = (String) map.get("keyB");
        DirContext ctx = null;
        SearchControls sc = new SearchControls();
        String filter = "(uid=" + bar + ")";
        ctx.search("ou=users", filter, sc);
    }
}
`
	runLDAPPropagationCase(t, src)
}

// TestLDAP_URLDecodePropagation documents that taint does not propagate
// through URLDecoder.decode.
//
// Reproduces OWASP Benchmark LDAP cases (e.g. BenchmarkTest00012):
//
//	String param = request.getHeaders("...").nextElement();
//	param = java.net.URLDecoder.decode(param, "UTF-8");
//	String filter = "...(uid=" + param + ")...";
//	idc.search(base, filter, filters, sc);
//
// Taint should flow: param -> URLDecoder.decode -> param (reassigned) ->
// filter -> search. Currently it does NOT — the SSA pipeline loses taint
// through the URLDecoder.decode call.
func TestLDAP_URLDecodePropagation(t *testing.T) {
	skipXPathPropagation(t, "URLDecoder.decode: taint through URLDecoder.decode(param, \"UTF-8\") is not tracked to the reassigned variable")
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.naming.directory.DirContext;
import javax.naming.directory.SearchControls;
import java.net.URLDecoder;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        param = URLDecoder.decode(param, "UTF-8");
        DirContext ctx = null;
        SearchControls sc = new SearchControls();
        String filter = "(uid=" + param + ")";
        ctx.search("ou=users", filter, sc);
    }
}
`
	runLDAPPropagationCase(t, src)
}