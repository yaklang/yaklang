package syntaxflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// These tests document SSA taint-propagation behaviour observed while triaging
// irify-benchmark securibench-micro XSS false negatives (Aliasing3 / Aliasing5).
//
// Context: the XSS rule fires correctly on direct getParameter -> println
// flows (confirmed via end-to-end `go run common/yak/cmd/yak.go code-scan`).
// Each case below isolates one aliasing shape and states what the SSA/SyntaxFlow
// pipeline *should* do for it.
//
// Classification (verified against Java semantics):
//   - DirectPropagation        : true vuln, must fire.    (control)
//   - ParameterValuesIndex     : true vuln, must fire.    (no special handling
//                                needed; SSA folds names[0] into the array
//                                value and the rule's include-filter matches)
//   - ArrayElementSwap         : NOT a real vuln, must NOT fire. (Java value
//                                semantics: str is bound to the array's value
//                                at read time, before the tainted write)
//   - InterproceduralAlias     : true vuln, currently does NOT fire. Real SSA
//                                gap — interprocedural mutator side-effect.

// servletXSSPropagationRule mirrors the shape of the real java-servlet-xss.sf
// rule but stripped to the minimum needed to exercise propagation. It binds a
// source ($source), a sink ($sink) and requires the sink to be reachable from
// the source via an include-filter ($tainted). Pass == propagation works;
// empty $tainted == propagation gap.
//
// Note: the include-filter must use backtick-quoted filter syntax with `&`
// (set intersection), per the SyntaxFlow DSL. The earlier failure with `* &`
// was a rule-syntax issue, not an SSA one.
var servletXSSPropagationRule = strings.NewReplacer(
	"\t", " ",
).Replace(strings.TrimSpace(`
*?{opcode:param}?{<typeName>?{have:'HttpServletRequest'}} as $req;
$req.getParameter() as $source;
*.getWriter()?{<getCallee><typeName>?{have:'HttpServletResponse'}} as $out;
$out.println(,* as $sink);
$sink#{include: "* & $source"}-> as $tainted;
alert $tainted for { title: "propagation test" }
`))

// runPropagationCase compiles src as Java, runs the propagation rule, and
// asserts that the $tainted alert fires (i.e. taint reached the sink).
func runPropagationCase(t *testing.T, src string) {
	t.Helper()
	prog, err := ssaapi.Parse(strings.TrimSpace(src),
		ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(servletXSSPropagationRule)
	require.NoError(t, err)
	require.NotNil(t, res)
	// $tainted is the alert variable; non-empty means the rule fired.
	tainted := res.GetValues("tainted")
	if len(tainted) == 0 {
		t.Fatalf("taint did not propagate: $tainted is empty (source/sink both matched, propagation gap)")
	}
}

// assertNoPropagation is the inverse of runPropagationCase: it asserts that
// $tainted does NOT fire, i.e. the SSA pipeline correctly concludes there is
// no taint flow. Used for shapes that look dangerous but are safe by language
// semantics.
func assertNoPropagation(t *testing.T, src string) {
	t.Helper()
	prog, err := ssaapi.Parse(strings.TrimSpace(src),
		ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(servletXSSPropagationRule)
	require.NoError(t, err)
	require.NotNil(t, res)
	if tainted := res.GetValues("tainted"); len(tainted) != 0 {
		t.Fatalf("expected NO taint flow but $tainted fired %d time(s) — false positive", len(tainted))
	}
}

// skipPropagation marks a test as a known SSA propagation gap.
func skipPropagation(t *testing.T, reason string) {
	t.Helper()
	t.Skipf("SSA taint-propagation gap (benchmark FN evidence): %s\n"+
		"Unskip after improving taint propagation in common/yak/ssa.", reason)
}

var _ = skipPropagation // retained for future gap tests; currently unused

// TestServletXSS_DirectPropagation is the control case: getParameter result
// flows directly to println. Must pass — it proves the rule + source lib are
// correct before testing the harder propagation shapes.
func TestServletXSS_DirectPropagation(t *testing.T) {
	const src = `
package x;
import java.io.PrintWriter;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String name = req.getParameter("name");
        PrintWriter w = resp.getWriter();
        w.println(name);
    }
}
`
	runPropagationCase(t, src)
}

// TestServletXSS_ArrayElementSwapPropagation is the Aliasing3 shape. It is NOT
// a real XSS vuln under Java value semantics:
//
//	String name = req.getParameter("name");   // tainted
//	String[] a  = new String[10];             // a[*] == null
//	String str  = a[5];                        // str := null  (value copied NOW)
//	a[5] = name;                              // mutates a[5] only, not str
//	name = str;                                // tainted value of name is dropped
//	writer.println(str);                       // prints null — no taint reaches sink
//
// The earlier read of `a[5]` is not affected by the later write: Java arrays
// hold values, and `str = a[5]` snapshots the element. SSA correctly folds
// `str` to the array's default value (nil), so no taint flows to println.
//
// securibench-micro labels this `vuln_count=1` and marks the println `BAD`,
// which is a ground-truth annotation error (or a deliberately misleading
// sample). The SSA/SyntaxFlow pipeline must NOT alert here — alerting would
// be a false positive. We assert no propagation instead of skipping.
func TestServletXSS_ArrayElementSwapPropagation(t *testing.T) {
	const src = `
package x;
import java.io.PrintWriter;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String name = req.getParameter("name");
        String[] a = new String[10];
        String str = a[5];
        a[5] = name;
        name = str;
        PrintWriter w = resp.getWriter();
        w.println(str);
    }
}
`
	assertNoPropagation(t, src)
}

// TestServletXSS_InterproceduralAliasPropagation documents that taint does not
// propagate through an interprocedural alias where a StringBuffer is appended
// with tainted data in a callee and printed from a different alias of the same
// object in the caller.
//
// Reproduces securibench-micro Aliasing5: `foo(buf, buf, resp, req)` passes
// the same StringBuffer twice; the callee appends req.getParameter(...) to it
// and the caller prints buf2.toString(). Verified Risk=0 even when callee
// formal params are typed as HttpServletRequest/Response (so the gap is in
// interprocedural alias propagation, not the source type filter).
func TestServletXSS_InterproceduralAliasPropagation(t *testing.T) {
	skipPropagation(t, "interprocedural object aliasing: taint appended to a shared StringBuffer in a callee is not seen at the caller's alias")
	const src = `
package x;
import java.io.PrintWriter;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        StringBuffer buf = new StringBuffer("abc");
        foo(buf, buf, resp, req);
    }
    void foo(StringBuffer buf, StringBuffer buf2, HttpServletResponse resp, HttpServletRequest req) throws Exception {
        String name = req.getParameter("name");
        buf.append(name);
        PrintWriter w = resp.getWriter();
        w.println(buf2.toString());
    }
}
`
	runPropagationCase(t, src)
}

// TestServletXSS_ParameterValuesIndex covers the `getParameterValues()[i]`
// shape (securibench-micro Aliasing6 family). A true vuln: the array itself
// is the source, and any index read carries taint.
//
// Earlier this was t.Skip'd as a "suspected index-read propagation gap", but
// the gap did not reproduce once the rule's source is hooked to
// getParameterValues (the source-lib fix in the same change-set). SSA does
// not emit a distinct index-read instruction here — it keeps `names` as the
// tainted value and the rule's include-filter (`* & $source`) matches because
// the println argument is dominated by the tainted array. So this case now
// passes unconditionally.
//
// Note: the surrounding servletXSSPropagationRule uses `$req.getParameter()`
// as $source, which does not match this sample (it uses getParameterValues).
// We run a rule variant below that hooks the correct source so the assertion
// reflects the real scenario.
func TestServletXSS_ParameterValuesIndexPropagation(t *testing.T) {
	const src = `
package x;
import java.io.PrintWriter;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
class C {
    void doGet(HttpServletRequest req, HttpServletResponse resp) throws Exception {
        String[] names = req.getParameterValues("name");
        PrintWriter w = resp.getWriter();
        w.println(names[0]);
    }
}
`
	// Variant rule: source = getParameterValues instead of getParameter.
	rule := strings.NewReplacer("\t", " ").Replace(strings.TrimSpace(`
*?{opcode:param}?{<typeName>?{have:'HttpServletRequest'}} as $req;
$req.getParameterValues() as $source;
*.getWriter()?{<getCallee><typeName>?{have:'HttpServletResponse'}} as $out;
$out.println(,* as $sink);
$sink#{include: "* & $source"}-> as $tainted;
alert $tainted
`))
	prog, err := ssaapi.Parse(strings.TrimSpace(src),
		ssaapi.WithLanguage(ssaconfig.JAVA))
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(rule)
	require.NoError(t, err)
	require.NotEmpty(t, res.GetValues("tainted"),
		"getParameterValues()[i] must reach sink: $tainted empty")
}
