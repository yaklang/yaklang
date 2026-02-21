package java

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const logForgingFullRule = `
<include("java-servlet-param")> as $source;
<include("java-spring-mvc-param")> as $source;
<include("java-log-record")> as $log;
$log#{include:` + "`* & $source`" + `}-> as $dest;
$dest<getPredecessors> as $sink;
$sink as $result;
`

const logForgingCoreRule = `
request.getParameter(* as $source);
log.error(* as $log);
log.info(* as $log);
log.warn(* as $log);
$log#{include:` + "`* & $source`" + `}-> as $dest;
$dest<getPredecessors> as $sink;
$sink as $result;
`

func buildLogForgingPerfJavaSource(methods, wrapDepth int) string {
	var sb strings.Builder
	sb.WriteString("package com.example;\n")
	sb.WriteString("import javax.servlet.http.HttpServletRequest;\n")
	sb.WriteString("import org.slf4j.Logger;\n")
	sb.WriteString("import org.slf4j.LoggerFactory;\n")
	sb.WriteString("public class LogForgingPerf {\n")
	sb.WriteString("  private static final Logger log = LoggerFactory.getLogger(LogForgingPerf.class);\n")
	sb.WriteString("  private String wrap0(String v) { return v; }\n")
	for i := 1; i <= wrapDepth; i++ {
		fmt.Fprintf(&sb, "  private String wrap%d(String v) { return wrap%d(v) + \"_%d\"; }\n", i, i-1, i)
	}
	for i := 0; i < methods; i++ {
		fmt.Fprintf(&sb, "  public void process%d(HttpServletRequest request) {\n", i)
		fmt.Fprintf(&sb, "    String p%d = request.getParameter(\"p%d\");\n", i, i)
		fmt.Fprintf(&sb, "    String x%d = wrap%d(p%d);\n", i, wrapDepth, i)
		fmt.Fprintf(&sb, "    if ((x%d.length() & 1) == 0) { log.info(\"info%d={} \", x%d); } else { log.error(\"err%d=\" + x%d); }\n", i, i, i, i, i)
		sb.WriteString("  }\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}

func buildLogForgingProgram(b testing.TB) *ssaapi.Program {
	b.Helper()
	code := buildLogForgingPerfJavaSource(80, 8)
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.JAVA))
	if err != nil {
		b.Fatalf("parse java code failed: %v", err)
	}
	return prog
}

func runLogForgingRule(b testing.TB, prog *ssaapi.Program, rule string) int {
	b.Helper()
	res, err := prog.SyntaxFlowWithError(rule)
	if err != nil {
		b.Fatalf("run syntaxflow failed: %v", err)
	}
	n := res.GetValues("result").Len()
	if n <= 0 {
		b.Fatalf("empty result for log forging rule")
	}
	return n
}

func TestLogForgingRulePerfRegression(t *testing.T) {
	prog := buildLogForgingProgram(t)
	base := runLogForgingRule(t, prog, logForgingFullRule)
	for i := 0; i < 3; i++ {
		got := runLogForgingRule(t, prog, logForgingFullRule)
		if got != base {
			t.Fatalf("round %d result changed: base=%d got=%d", i+1, base, got)
		}
	}
}

func BenchmarkLogForgingRuleFullPerf(b *testing.B) {
	prog := buildLogForgingProgram(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = runLogForgingRule(b, prog, logForgingFullRule)
	}
}

func BenchmarkLogForgingRuleCoreTopDefPerf(b *testing.B) {
	prog := buildLogForgingProgram(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = runLogForgingRule(b, prog, logForgingCoreRule)
	}
}

