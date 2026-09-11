package ssaapi

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestValidRuleModeStructIsNotSSA(t *testing.T) {
	require.Equal(t, schema.SFR_MODE_STRUCT, schema.ValidRuleMode("struct"))
	require.Equal(t, schema.SFR_MODE_SSA, schema.ValidRuleMode("unknown"))
}

func TestStructRuleRejectedOnBareProgram(t *testing.T) {
	prog, err := Parse("a = 1")
	require.NoError(t, err)
	rule := &schema.SyntaxFlowRule{
		RuleName: "struct-rule",
		Mode:     schema.SFR_MODE_STRUCT,
		Language: ssaconfig.General,
		Content: `desc(mode: "struct", language: "yak")
a as $hit
alert $hit
`,
	}
	_, err = prog.SyntaxFlowRule(rule)
	require.Error(t, err)
	require.Contains(t, err.Error(), "QueryWithStruct")
}

func TestStructRuleContentRejectedWithoutQueryWithStruct(t *testing.T) {
	prog, err := Parse("a = 1")
	require.NoError(t, err)
	_, err = prog.SyntaxFlowWithError(`
desc(mode: "struct")
a as $hit
alert $hit
`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "struct rule requires QueryWithStruct")
}

func TestUnknownRuleModeExecutedAsSSA(t *testing.T) {
	prog, err := Parse("a = 1")
	require.NoError(t, err)
	res, err := prog.SyntaxFlowWithError(`
desc(mode: "not-a-real-mode")
a as $hit
alert $hit
`)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(res.GetValues("hit")), 1)
}

func TestFrameIsStructModeFromDesc(t *testing.T) {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(`
desc(mode: "struct", language: "java")
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call
`)
	require.NoError(t, err)
	require.True(t, sfvm.FrameIsStructMode(frame))
	require.False(t, sfvm.FrameIsSourceMode(frame))
}

// TestStructScanFindsCallAndDoesNotCrossPackage compiles two Java packages
// with struct rules. Scan runs after each SCC's LazyBuildForUnits (method
// bodies resident) and before FlushCompileUnit — not after Application.Finish.
func TestStructScanFindsCallAndDoesNotCrossPackage(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/Sink.java", `package a;
class Sink {
  void f(String cmd) throws Exception {
    Runtime.getRuntime().exec(cmd);
  }
}
`)
	vf.AddFile("b/Src.java", `package b;
class Src {
  static String taint() { return "x"; }
  void g(String cmd) throws Exception {
    Runtime.getRuntime().exec(b.Src.taint());
  }
}
`)
	progName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

	callRule := `
desc(
    mode: "struct"
    language: "java"
    title: "Runtime.exec"
    level: medium
    risk: "command-injection"
)
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call for {
    title: "Runtime.exec"
}
`
	taintRule := `
desc(
    mode: "struct"
    language: "java"
    title: "cross-package taint should not fire"
)
Runtime.getRuntime().exec(* as $cmd) as $call
$cmd #-> * as $src
alert $src
`

	var risks []*schema.SSARisk
	progs, err := ParseProjectWithFS(vf,
		WithLanguage(ssaconfig.JAVA),
		WithProgramName(progName),
		WithStructRuleRaw(callRule),
		WithStructRuleRaw(taintRule),
		WithStructRuleCallback(func(risk *schema.SSARisk) {
			risks = append(risks, risk)
		}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	var callHits int
	for _, risk := range risks {
		if risk.Title == "Runtime.exec" {
			callHits++
			require.True(t, strings.Contains(risk.CodeSourceUrl, "Sink.java") || strings.Contains(risk.CodeSourceUrl, "Src.java"))
		}
	}
	require.GreaterOrEqual(t, callHits, 2, "should hit Runtime.exec in both packages")

	var unitA *CompileUnit
	for _, unit := range progs[0].Program.CompileUnits {
		if unit != nil && unit.Key == "java:a" {
			unitA = unit
			break
		}
	}
	require.NotNil(t, unitA)
	res, err := QuerySyntaxflow(
		QueryWithValue(NewStructQueryTarget(progs[0], unitA, nil)),
		QueryWithResultProgram(progs[0]),
		QueryWithStruct(unitA),
		QueryWithRuleContent(taintRule),
	)
	require.NoError(t, err)
	for _, name := range res.GetAlertVariables() {
		for _, val := range res.GetValues(name) {
			if val == nil || val.GetRange() == nil || val.GetRange().GetEditor() == nil {
				continue
			}
			require.NotContains(t, val.GetRange().GetEditor().GetUrl(), "Src.java",
				"scanning package a must not follow taint into package b")
		}
	}
}

func TestQueryWithStructRejectsSSARule(t *testing.T) {
	prog, err := Parse("a = 1")
	require.NoError(t, err)
	unit := &CompileUnit{Key: "unit:test"}
	_, err = QuerySyntaxflow(
		QueryWithValue(NewStructQueryTarget(prog, unit, nil)),
		QueryWithResultProgram(prog),
		QueryWithStruct(unit),
		QueryWithRuleContent(`
desc(mode: "ssa")
a as $hit
alert $hit
`),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "QueryWithStruct only accepts struct rules")
}

func TestStructScanPythonEval(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/app.py", "def f(x):\n    eval(x)\n")
	vf.AddFile("b/ok.py", "def g():\n    return 1\n")

	progName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), progName)

	var risks []*schema.SSARisk
	progs, err := ParseProjectWithFS(vf,
		WithLanguage(ssaconfig.PYTHON),
		WithProgramName(progName),
		WithStructRuleRaw(`
desc(
    mode: "struct"
    language: "python"
    title: "eval"
)
eval(* as $arg) as $call
alert $call for {
    title: "eval"
}
`),
		WithStructRuleCallback(func(risk *schema.SSARisk) {
			risks = append(risks, risk)
		}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	require.GreaterOrEqual(t, len(risks), 1)
	for _, risk := range risks {
		require.Contains(t, risk.CodeSourceUrl, "app.py")
		require.NotContains(t, risk.CodeSourceUrl, "ok.py")
	}
}

func TestStructScanBuiltinSnippets(t *testing.T) {
	cases := []struct {
		name     string
		lang     ssaconfig.Language
		file     string
		code     string
		rule     string
		ssaQuery string
	}{
		{
			name: "python-eval",
			lang: ssaconfig.PYTHON,
			file: "dyn.py",
			code: "def run(user_input):\n    return eval(user_input)\n",
			rule: `desc(mode: "struct", language: "python")
eval(* as $arg) as $call
alert $call`,
			ssaQuery: `eval(* as $arg) as $call
alert $call`,
		},
		{
			name: "python-os-system",
			lang: ssaconfig.PYTHON,
			file: "sh.py",
			code: "import os\ndef run(user):\n    os.system(\"ls \" + user)\n",
			rule: `desc(mode: "struct", language: "python")
os.system as $call
os.system(* as $arg) as $call
alert $call`,
			ssaQuery: `os.system as $call
alert $call`,
		},
		{
			name: "python-pickle",
			lang: ssaconfig.PYTHON,
			file: "ser.py",
			code: "import pickle\ndef load(data):\n    return pickle.loads(data)\n",
			rule: `desc(mode: "struct", language: "python")
pickle.loads as $call
alert $call`,
			ssaQuery: `pickle.loads as $call
alert $call`,
		},
		{
			name: "java-md5",
			lang: ssaconfig.JAVA,
			file: "Hash.java",
			code: `import java.security.MessageDigest;
class Hash {
  void bad() throws Exception {
    MessageDigest.getInstance("MD5");
  }
}
`,
			rule: `desc(mode: "struct", language: "java")
MessageDigest.getInstance(*?{have: "MD5"}) as $call
alert $call`,
			ssaQuery: `MessageDigest.getInstance(*?{have: "MD5"}) as $call
alert $call`,
		},
		{
			name: "js-eval",
			lang: ssaconfig.JS,
			file: "app.js",
			code: "function run(code) { return eval(code); }\n",
			rule: `desc(mode: "struct", language: "javascript")
eval(* as $arg) as $call
alert $call`,
			ssaQuery: `eval(* as $arg) as $call
alert $call`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vf := filesys.NewVirtualFs()
			vf.AddFile(tc.file, tc.code)
			progName := uuid.NewString()
			defer ssadb.DeleteProgram(ssadb.GetDB(), progName)
			progs, err := ParseProjectWithFS(vf,
				WithLanguage(tc.lang),
				WithProgramName(progName),
				WithMemory(),
				WithStructRuleRaw(tc.rule),
			)
			require.NoError(t, err)
			require.NotEmpty(t, progs)
			t.Logf("struct results=%d errs=%v", len(progs[0].StructScanResults()), progs[0].StructScanErrors())
			for _, res := range progs[0].StructScanResults() {
				t.Logf("struct alerts=%v", res.GetAlertVariables())
			}
			ssaRes, err := progs[0].SyntaxFlowWithError(tc.ssaQuery)
			require.NoError(t, err)
			t.Logf("ssa alerts=%v count=%d", ssaRes.GetAlertVariables(), len(ssaRes.GetAlertVariables()))
			for _, name := range ssaRes.GetAlertVariables() {
				for _, val := range ssaRes.GetValues(name) {
					if val == nil {
						continue
					}
					fp := ""
					if val.GetRange() != nil && val.GetRange().GetEditor() != nil {
						fp = val.GetRange().GetEditor().GetUrl()
					}
					t.Logf("ssa hit opcode=%s name=%s file=%s", val.GetOpcode(), val.GetName(), fp)
				}
			}
			structHits := 0
			for _, res := range progs[0].StructScanResults() {
				for _, name := range res.GetAlertVariables() {
					structHits += len(res.GetValues(name))
				}
			}
			require.Greater(t, structHits, 0, "struct scan should hit")
		})
	}
}

func TestStructScanRequiresProgramName(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; class A { void f() {} }`)
	_, err := ParseProjectWithFS(vf,
		WithLanguage(ssaconfig.JAVA),
		WithStructRuleRaw(`
desc(mode: "struct", language: "java")
A as $hit
alert $hit
`),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "withProgramName")
}
