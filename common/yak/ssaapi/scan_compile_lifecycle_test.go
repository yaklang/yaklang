package ssaapi

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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
		WithProgramName(uuid.NewString()), WithMemory(), WithStructRules(rule))
	require.NoError(t, err)
	require.Len(t, progs, 1)
	require.NotEmpty(t, rule.OpCodes)
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

func TestStructScanReleasesPersistedBatchResults(t *testing.T) {
	t.Setenv(compileUnitBatchMinFilesEnv, "1")
	t.Setenv(compileUnitBatchMinBytesEnv, "1")
	t.Setenv("YAK_SSA_COMPILE_UNIT_LOG", "1")
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; class A { void f(String s) throws Exception { Runtime.getRuntime().exec(s); } }`)
	vf.AddFile("b/B.java", `package b; class B { void f(String s) throws Exception { Runtime.getRuntime().exec(s); } }`)
	name := uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), name) })
	var state *structScanRuntime
	callbacks := 0
	completedBatches := 0
	progs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.JAVA), WithProgramName(name),
		ssaconfig.WithCompileIrCacheTTL(time.Millisecond), ssaconfig.WithCompileIrCacheMax(1),
		WithStructRuleRaw(`desc(mode: "struct", language: "java")
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call`),
		ssaconfig.SetOption("test/struct-state", func(c *Config, _ bool) {
			state = c.ensureStructScan()
		})(true),
		WithProcess(func(msg string, _ float64) {
			if strings.Contains(msg, "build finished units=") {
				completedBatches++
				require.NotEmpty(t, state.results)
				require.Zero(t, state.results[0].program.Program.Cache.GetFlushAccounting().Pending,
					"instruction persistence must settle before a batch is declared complete")
			}
		}),
		WithStructRuleCallback(func(*schema.SSARisk) {
			callbacks++
			require.True(t, state.results[len(state.results)-1].program.Program.Cache.IsInstructionSpillDisabled(),
				"completed batch IR must remain resident until its struct scan finishes")
			if callbacks == 2 {
				require.Equal(t, 1, state.persisted, "previous batch must persist before the next batch scans")
				require.Nil(t, state.results[0].memResult, "previous VM frame must no longer retain its Value graph")
				require.Empty(t, state.results[0].symbol)
			}
		}))
	require.NoError(t, err)
	require.Equal(t, 2, callbacks)
	require.Equal(t, 2, completedBatches)
	require.Empty(t, progs[0].StructScanErrors())
	require.Equal(t, 2, state.persisted)
	for _, result := range progs[0].StructScanResults() {
		require.Nil(t, result.memResult)
		require.NotNil(t, result.dbResult)
		require.Equal(t, 1, result.RiskCount())
		values := result.GetValues("call")
		require.Len(t, values, 1, "persisted alert must remain readable after compilation")
		require.NotNil(t, values[0].GetRange())
	}
}

func TestStructScanSplitsCyclicUnitsWithoutLosingDataflow(t *testing.T) {
	t.Setenv(compileUnitBatchMaxFilesEnv, "1")
	t.Setenv("YAK_SSA_COMPILE_UNIT_LOG", "1")
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/A.java", `package a; import b.B;
class A { static String value() { return B.value(); } }`)
	vf.AddFile("b/B.java", `package b; import a.A;
class B {
  static String value() { return "cyclic-batch-value"; }
  static void run() { println(A.value()); }
}`)
	name := uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), name) })
	batches := 0
	progs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.JAVA), WithProgramName(name),
		WithStructRuleRaw(`desc(mode: "struct", language: "java")
println(* as $arg)
alert $arg`),
		WithProcess(func(msg string, _ float64) {
			if strings.Contains(msg, "build finished units=") {
				batches++
			}
		}))
	require.NoError(t, err)
	require.Equal(t, 2, batches, "struct scans must preserve the batch size limit inside an SCC")
	require.Empty(t, progs[0].StructScanErrors())
	loaded, err := FromDatabase(name)
	require.NoError(t, err)
	result, err := loaded.SyntaxFlowWithError(`println(* #-> * as $source)`)
	require.NoError(t, err)
	require.Contains(t, result.GetValues("source").String(), "cyclic-batch-value")
}
