package sfanalysis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEvaluateVerifyFilesystemWithRule_BlankIsNoop(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: " \n\t "}
	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule))
	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()))
}

func TestEvaluateVerifyFilesystemWithRule_DefaultSkipsNegativeFilesystem(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	language: yaklang,
	'file://unsafe.yak': <<<EOF
a = 1;
EOF
	'safe://safe.yak': <<<EOF
a = 1;
EOF
)

a as $output;
alert $output;
`}

	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule))
	require.ErrorContains(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()), "alert symbol table not empty")
}

func TestEvaluateVerifyFilesystemWithRule_DefaultAllowsAlertHighOverflow(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	language: yaklang,
	alert_high: 1,
	'file://unsafe.yak': <<<EOF
a = 1;
EOF
	'safe://safe.yak': <<<EOF
b = 1;
EOF
)

a as $first;
a as $second;
alert $first for {level: "high"};
alert $second for {level: "high"};
`}

	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule))
	require.ErrorContains(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()), "alert symbol table is less than alert_high config")
}

func TestEvaluateVerifyFilesystemWithRule_StructModeCompilesWithStructRule(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	mode: "struct"
	language: "java"
	alert_min: 1
	"file://Run.java": <<<POS
class Run {
  void bad(String cmd) throws Exception {
    Runtime.getRuntime().exec(cmd);
  }
}
POS
	"safefile://RunSafe.java": <<<NEG
class RunSafe {
  void ok() { System.out.println("no exec"); }
}
NEG
)
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call for { title: "Runtime.exec" }
`}
	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()))
}

func TestEvaluateVerifyFilesystemWithRule_SourceModeUsesFileSystem(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	mode: "source"
	language: "python"
	alert_min: 1
	"file://dyn.py": <<<POS
eval(user)
POS
	"safefile://dyn-safe.py": <<<NEG
print(user)
NEG
)
${*.py}.pattern_regex(/eval\s*\(/) as $call
alert $call
`}
	require.NoError(t, EvaluateVerifyFilesystemWithRule(rule, WithStrictEmbeddedVerify()))
}

func TestEvaluateVerifyFilesystemWithRule_SourceModeAlwaysChecksNegative(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	mode: "source"
	language: "python"
	alert_min: 1
	"file://dyn.py": <<<POS
eval(user)
POS
	"safefile://dyn-safe.py": <<<NEG
eval(user)
NEG
)
${*.py}.pattern_regex(/eval\s*\(/) as $call
alert $call
`}
	require.ErrorContains(t, EvaluateVerifyFilesystemWithRule(rule), "unexpected alert")
}

func TestEvaluateVerifyFilesystemWithRule_StructModeAlwaysChecksNegative(t *testing.T) {
	rule := &schema.SyntaxFlowRule{Content: `
desc(
	mode: "struct"
	language: "java"
	alert_min: 1
	"file://Run.java": <<<POS
class Run {
  void bad(String cmd) throws Exception {
    Runtime.getRuntime().exec(cmd);
  }
}
POS
	"safefile://RunSafe.java": <<<NEG
class RunSafe {
  void ok(String cmd) throws Exception {
    Runtime.getRuntime().exec(cmd);
  }
}
NEG
)
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call for { title: "Runtime.exec" }
`}
	require.ErrorContains(t, EvaluateVerifyFilesystemWithRule(rule), "alert symbol table not empty")
}

func TestBuiltinStructRules_VerifyFilesystem(t *testing.T) {
	RunBuiltinRuleVerify(t, BuiltinVerifyFilter{Struct: true})
}
