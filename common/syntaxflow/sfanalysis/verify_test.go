package sfanalysis

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
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

func TestBuiltinStructRules_VerifyFilesystem(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(thisFile), "..", "sfbuildin", "buildin", "struct")
	root, err := filepath.Abs(root)
	require.NoError(t, err)

	var rules []string
	err = filesys.Recursive(root, filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".sf") {
			return nil
		}
		rules = append(rules, path)
		return nil
	}))
	require.NoError(t, err)
	require.NotEmpty(t, rules, "expected struct/*.sf under %s", root)

	local := filesys.NewLocalFs()
	for _, path := range rules {
		path := path
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			raw, err := local.ReadFile(path)
			require.NoError(t, err)
			frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(string(raw))
			require.NoError(t, err)
			require.True(t, sfvm.FrameIsStructMode(frame), "rule must be mode=struct")
			err = EvaluateVerifyFilesystemWithFrame(frame, WithStrictEmbeddedVerify())
			require.NoError(t, err)
		})
	}
}
