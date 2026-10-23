package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestExtractUnitKeyFromFunctionKey(t *testing.T) {
	// Java style: lang:package.Class.method
	require.Equal(t, "java:com.example", extractUnitKeyFromFunctionKey("java:com.example.Service.handle"))
	// Two parts keep the whole key (legacy behaviour).
	require.Equal(t, "java:Main.run", extractUnitKeyFromFunctionKey("java:Main.run"))
	// Bare names are the only thing this parser cannot resolve; the recorded
	// compile unit is used for those instead (see the test below).
	require.Equal(t, "", extractUnitKeyFromFunctionKey("LivyClient___init__"))
}

// A Python method is named "<ClassName>_<method>", so it starts with an upper
// case letter even though it is not a top-level cross-unit symbol. Treating it
// as public kept every method body resident for the whole program.
func TestShouldKeepFunctionForCrossUnitReferenceTreatsMethodsAsNonPublic(t *testing.T) {
	prog := newTestProgramForUnitRelease(t, "method-visibility")
	fn := prog.NewFunction("LivyClient___init__")
	fn.SetMethodName("__init__")
	require.False(t, shouldKeepFunctionForCrossUnitReference(fn),
		"a class method must not be treated as a top-level cross-unit symbol")

	exported := prog.NewFunction("ExportedFunction")
	require.True(t, shouldKeepFunctionForCrossUnitReference(exported),
		"a plain exported function must still be kept")
	require.NoError(t, prog.Cache.SaveToDatabase())
}

// The body release must not depend on parsing a language-specific function key:
// non-Java function names carry no "lang:" prefix, so the unit recorded at
// creation time is the only reliable match.
func TestReleaseCompletedUnitMemoryUsesRecordedUnitKey(t *testing.T) {
	prog := newTestProgramForUnitRelease(t, "recorded-unit-key")
	prog.BeginCompileUnit("dir:pkg")
	fn := prog.NewFunction("helper")
	builder := prog.GetAndCreateFunctionBuilder("", "helper")
	_ = fn
	builder.Finish()
	prog.EndCompileUnit()

	require.NotEmpty(t, fn.compileUnitKey, "function must record its compile unit")
	require.NotEmpty(t, fn.Blocks, "finished function must have blocks before release")

	released := prog.ReleaseCompletedUnitMemory([]string{"dir:pkg"})
	require.Greater(t, released, 0, "function of a completed unit must be released")
	require.NotEmpty(t, fn.Blocks, "block ids must stay until scope values are unlinked")
	prog.ReleasePersistedOrdinaryValues()
	require.Empty(t, fn.Blocks, "private function block ids are dropped after the scope walk")
	require.NoError(t, prog.Cache.SaveToDatabase())
}

func newTestProgramForUnitRelease(t *testing.T, name string) *Program {
	t.Helper()
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(name))
	require.NoError(t, err)
	cfg.SetCompileUnitSplit(true)
	return NewProgram(cfg, ProgramCacheDBWrite, Application, nil, "", 1)
}

// Finish must tolerate a body that was already released. ReleaseCompletedUnitMemory
// only drops finished bodies, but a stale unit key or a lazily registered function
// must never turn that into an index-out-of-range panic that discards the program.
func TestFunctionFinishWithoutBlocksDoesNotPanic(t *testing.T) {
	prog := newTestProgramForUnitRelease(t, "finish-empty-blocks")
	builder := prog.GetAndCreateFunctionBuilder("", "helper")
	builder.Function.Blocks = nil
	require.NotPanics(t, func() { builder.Function.Finish() })
	require.NoError(t, prog.Cache.SaveToDatabase())
}
