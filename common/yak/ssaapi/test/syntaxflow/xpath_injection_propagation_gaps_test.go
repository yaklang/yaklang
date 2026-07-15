package syntaxflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// These tests document SSA taint-propagation gaps observed while triaging
// irify-benchmark OWASP Benchmark XPath injection (CWE-643) false negatives.
//
// Context: the XPath rule fires correctly on direct getParameter ->
// xp.evaluate/xp.compile flows (confirmed via end-to-end `go run
// common/yak/cmd/yak.go code-scan`). 13 of 15 true-positive cases miss
// because taint is lost on the way from source to sink. Each case below
// isolates one propagation shape and states what the SSA/SyntaxFlow pipeline
// *should* do.
//
// Classification (verified against OWASP Benchmark v1.2 expected results):
//   - DirectPropagation        : true vuln, must fire.    (control)
//   - InterproceduralReturn     : true vuln, currently does NOT fire. SSA gap —
//                                 taint passed to a method parameter and
//                                 returned is not tracked back to the caller.
//   - Base64EncodeDecode       : true vuln, currently does NOT fire. SSA gap —
//                                 taint through Base64.encodeBase64 -> decodeBase64
//                                 -> new String(...) chain is not tracked.
//   - ConstantConditionSafe     : NOT a real vuln, must NOT fire. SSA should
//                                 evaluate compile-time-constant branch
//                                 conditions and exclude unreachable tainted
//                                 assignments. (FP triage)

// xpathPropagationRule mirrors the real java-direct-xpath-injection.sf rule
// but stripped to the minimum needed to exercise propagation. It binds a
// source ($source), a sink ($sink) for both .evaluate and .compile, and
// requires the sink to be reachable from the source via an include-filter.
var xpathPropagationRule = strings.NewReplacer(
	"\t", " ",
).Replace(strings.TrimSpace(`
*?{opcode:param}?{<typeName>?{have:'HttpServletRequest'}} as $req;
$req.getParameter() as $source;
.evaluate?{<typeName>?{have:'javax.xml.xpath.XPath'}}(* as $sink);
.compile?{<typeName>?{have:'javax.xml.xpath.XPath'}}(* as $sink);
check $sink;
$sink#{include: "<self> & $source"}-> as $tainted;
alert $tainted for { title: "xpath propagation test" }
`))

// runXPathPropagationCase compiles src as Java, runs the propagation rule,
// and asserts that the $tainted alert fires (taint reached the sink).
func runXPathPropagationCase(t *testing.T, src string) {
	t.Helper()
	prog, err := ssaapi.Parse(strings.TrimSpace(src),
		ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(xpathPropagationRule)
	require.NoError(t, err)
	require.NotNil(t, res)
	tainted := res.GetValues("tainted")
	if len(tainted) == 0 {
		t.Fatalf("taint did not propagate: $tainted is empty (source/sink both matched, propagation gap)")
	}
}

// assertNoXPathPropagation is the inverse: asserts $tainted does NOT fire.
func assertNoXPathPropagation(t *testing.T, src string) {
	t.Helper()
	prog, err := ssaapi.Parse(strings.TrimSpace(src),
		ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(xpathPropagationRule)
	require.NoError(t, err)
	require.NotNil(t, res)
	if tainted := res.GetValues("tainted"); len(tainted) != 0 {
		t.Fatalf("expected NO taint flow but $tainted fired %d time(s) — false positive", len(tainted))
	}
}

// skipXPathPropagation marks a test as a known SSA propagation gap.
func skipXPathPropagation(t *testing.T, reason string) {
	t.Helper()
	t.Skipf("SSA taint-propagation gap (benchmark FN evidence): %s\n"+
		"Unskip after improving taint propagation in common/yak/ssa.", reason)
}

// TestXPath_DirectPropagation is the control case: getParameter result flows
// directly to xp.evaluate. Must pass — it proves the rule + source are correct.
func TestXPath_DirectPropagation(t *testing.T) {
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.xml.xpath.XPath;
import javax.xml.xpath.XPathFactory;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        XPath xp = XPathFactory.newInstance().newXPath();
        String expression = "/Employees/Employee[@emplid='" + param + "']";
        String result = xp.evaluate(expression, (Object) null);
    }
}
`
	runXPathPropagationCase(t, src)
}

// TestXPath_DirectCompile is a second control: getParameter result flows
// directly to xp.compile. Must pass — it proves the .compile sink expansion works.
func TestXPath_DirectCompile(t *testing.T) {
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.xml.xpath.XPath;
import javax.xml.xpath.XPathFactory;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        XPath xp = XPathFactory.newInstance().newXPath();
        String expression = "/Employees/Employee[@emplid='" + param + "']";
        xp.compile(expression);
    }
}
`
	runXPathPropagationCase(t, src)
}

// TestXPath_InterproceduralReturnPropagation documents that taint does not
// propagate through a method that receives a tainted parameter and returns it.
//
// Reproduces OWASP Benchmark BenchmarkTest01316 (and 10 similar cases):
//
//	String param = request.getParameter("...");
//	String bar = new Test().doSomething(request, param);
//	String expression = "...'" + bar + "'...";
//	String result = xp.evaluate(expression, xmlDocument);
//
// The doSomething method simply assigns param to bar and returns it:
//
//	String bar;
//	if (condition) bar = param; else bar = "safe";
//	return bar;
//
// Taint should flow: param -> doSomething(param) -> return value -> bar ->
// expression -> xp.evaluate. Currently it does NOT — the return value of an
// interprocedural call is not tainted by the parameter's taint.
func TestXPath_InterproceduralReturnPropagation(t *testing.T) {
	skipXPathPropagation(t, "interprocedural return value: taint passed to a method parameter and returned is not tracked back to the caller's variable")
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.xml.xpath.XPath;
import javax.xml.xpath.XPathFactory;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        String bar = new Test().doSomething(req, param);
        XPath xp = XPathFactory.newInstance().newXPath();
        String expression = "/Employees/Employee[@emplid='" + bar + "']";
        String result = xp.evaluate(expression, (Object) null);
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
	runXPathPropagationCase(t, src)
}

// TestXPath_Base64EncodeDecodePropagation documents that taint does not
// propagate through a Base64 encode-then-decode chain.
//
// Reproduces OWASP Benchmark BenchmarkTest00207:
//
//	String param = request.getHeader("...");
//	param = java.net.URLDecoder.decode(param, "UTF-8");
//	String bar = new String(
//	    Base64.decodeBase64(Base64.encodeBase64(param.getBytes())));
//	String expression = "...'" + bar + "'...";
//	xp.evaluate(expression, xmlDocument);
//
// The Base64 encode-then-decode is a no-op semantically (the decoded output
// equals the original input), so taint should flow through. Currently it does
// NOT — the SSA pipeline does not track taint through the Base64 API calls.
func TestXPath_Base64EncodeDecodePropagation(t *testing.T) {
	skipXPathPropagation(t, "Base64 encode-decode chain: taint through Base64.encodeBase64 -> decodeBase64 -> new String(...) is not tracked")
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.xml.xpath.XPath;
import javax.xml.xpath.XPathFactory;
import org.apache.commons.codec.binary.Base64;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        String bar = new String(
            Base64.decodeBase64(
                Base64.encodeBase64(param.getBytes())));
        XPath xp = XPathFactory.newInstance().newXPath();
        String expression = "/Employees/Employee[@emplid='" + bar + "']";
        String result = xp.evaluate(expression, (Object) null);
    }
}
`
	runXPathPropagationCase(t, src)
}

// TestXPath_ConstantConditionSafe documents a false positive where the SSA
// pipeline does not evaluate compile-time-constant branch conditions.
//
// Reproduces OWASP Benchmark BenchmarkTest00117 (expected false positive):
//
//	int num = 86;
//	if ((7 * 42) - num > 200) bar = "This_should_always_happen";
//	else bar = param;  // dead branch: (7*42)-86 = 208 > 200 is always true
//	String expression = "...'" + bar + "'...";
//	xp.compile(expression).evaluate(...)
//
// (7*42)-86 = 208 > 200 is a compile-time constant true, so the else branch
// (bar = param) is dead code. bar is always the constant string, never tainted.
// The rule should NOT alert. Currently it DOES — the SSA pipeline treats both
// branches as possible, so bar is considered tainted from the else branch.
//
// This is a constant-condition evaluation gap. If the engine gains
// compile-time branch evaluation, this test should be flipped to
// assertNoXPathPropagation (expecting no alert). For now we document the FP.
func TestXPath_ConstantConditionSafe(t *testing.T) {
	t.Skipf("SSA constant-condition evaluation gap (benchmark FP evidence): %s\n"+
		"Unskip and switch to assertNoXPathPropagation after the SSA pipeline\n"+
		"evaluates compile-time-constant branch conditions and prunes dead branches.",
		"(7*42)-86 > 200 is always true but SSA treats both branches as reachable")
	const src = `
package x;
import javax.servlet.http.HttpServletRequest;
import javax.xml.xpath.XPath;
import javax.xml.xpath.XPathFactory;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String param = req.getParameter("name");
        String bar;
        int num = 86;
        if ((7 * 42) - num > 200) bar = "This_should_always_happen";
        else bar = param;
        XPath xp = XPathFactory.newInstance().newXPath();
        String expression = "/Employees/Employee[@emplid='" + bar + "']";
        xp.compile(expression);
    }
}
`
	// Currently the SSA pipeline treats both branches as reachable, so
	// taint flows from the else branch. This is the FP we document.
	// When the gap is fixed, switch to assertNoXPathPropagation.
	runXPathPropagationCase(t, src)
}