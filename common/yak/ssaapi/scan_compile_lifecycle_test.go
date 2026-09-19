package ssaapi

import (
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestTemporarySourceProgramsDoNotStartCacheWorkers(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		prog := NewTmpProgram("source")
		require.NotNil(t, prog.NewConstValue("source hit", nil))
		prog.ResetInterRuleState()
	}
	require.Less(t, runtime.NumGoroutine()-before, 50,
		"temporary source hits must not leave two cache workers per hit")
}

func TestStructScanReusesCompiledRuleAcrossUnits(t *testing.T) {
	rule, err := compileStructRuleContent(`desc(mode: "struct", language: "java")
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call`)
	require.NoError(t, err)
	require.NotEmpty(t, rule.OpCodes)
	rule.RuleName = "compiled-struct-rule"
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; class A { void f(String s) throws Exception { Runtime.getRuntime().exec(s); } }`)
	vf.AddFile("b/B.java", `package b; class B { void f(String s) throws Exception { Runtime.getRuntime().exec(s); } }`)
	progs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.JAVA),
		WithProgramName(uuid.NewString()), WithMemory(), WithStructRules(rule),
		WithStructRuleCallback(func(*schema.SSARisk) {
			// After the first unit, subsequent units must reuse in-process
			// compiled code, including in dev builds that reject DB opcodes.
			rule.Content = "this is not valid SyntaxFlow ((("
		}))
	require.NoError(t, err)
	require.Len(t, progs, 1)
	require.Empty(t, progs[0].StructScanErrors())
	require.Len(t, progs[0].StructScanResults(), 2)
	for _, result := range progs[0].StructScanResults() {
		require.Equal(t, rule.RuleName, result.GetRule().RuleName)
		require.NotEmpty(t, result.GetValues("call"))
	}
}

func TestStructScanLanguageSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lang   ssaconfig.Language
		ignore bool
		want   int
	}{
		{"java", ssaconfig.JAVA, false, 2},
		{"mixed", "", false, 3},
		{"ignore", ssaconfig.JAVA, true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &structScanRuntime{rules: []*schema.SyntaxFlowRule{
				{Language: ssaconfig.JAVA}, {Language: ssaconfig.PHP}, {Language: ssaconfig.General},
			}}
			s.filterLanguage(tc.lang, tc.ignore)
			require.Len(t, s.rules, tc.want)
		})
	}
}
