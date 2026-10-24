package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
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

// Finishing a private function and running a unit flush must release its
// builder so the function skeleton no longer pins CurrentBlock and its scope.
func TestReleaseCompletedUnitMemoryDropsPrivateFunctionBuilder(t *testing.T) {
	prog := newTestProgramForUnitRelease(t, "drop-private-builder")
	prog.BeginCompileUnit("dir:pkg")
	builder := prog.GetAndCreateFunctionBuilder("", "helper")
	fn := builder.Function
	builder.EmitConstInst(1)
	builder.Finish()
	prog.EndCompileUnit()

	require.NotNil(t, fn.builder)
	released := prog.ReleaseCompletedUnitMemory([]string{"dir:pkg"})
	require.Greater(t, released, 0)
	prog.ReleasePersistedOrdinaryValues()
	require.Nil(t, fn.builder, "private finished function must drop its builder after scope release")
	require.Empty(t, fn.Blocks)
	require.NoError(t, prog.Cache.SaveToDatabase())
}

// One-off ANTLR caches created by CreateAntlrCache must not stay pinned in
// PreHandlerBase.Caches once a worker cache is used: GetAntlrCache is called
// per worker and per external file, and keeping every cache in a slice retains
// every past batch's DFA until compile end.
func TestCreateAntlrCacheDoesNotAccumulateInPreHandlerBase(t *testing.T) {
	base := NewPreHandlerBase()
	cache := base.CreateAntlrCache(
		javaparser.GetJavaLexerSerializedATN(),
		javaparser.GetJavaParserSerializedATN(),
	)
	require.NotNil(t, cache)
	require.Empty(t, base.Caches, "CreateAntlrCache must not retain worker caches on the shared builder")
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

// A frontend may start a compile unit on a directory library and then create a
// separate package library inside the prehandler. Deferred tasks registered on
// that package library must inherit the same unit key so the batch runner
// drains and releases them instead of keeping them until compile end.
func TestBeginCompileUnitOnLibraryIsVisibleToSiblingLibrary(t *testing.T) {
	app := newTestProgramForUnitRelease(t, "unit-visible-across-libs")
	dirLib := app.EnsureLibraryForCompileUnit("dir:foo/bar")
	dirLib.BeginCompileUnit("dir:foo/bar")
	pkgLib := app.NewLibrary("bar", []string{"foo", "bar"})
	require.Equal(t, "dir:foo/bar", pkgLib.CurrentCompileUnit())
	ran := false
	pkgLib.RegisterDeferredBuild(DeferredBuildKindBinding, "test-bind", func() {
		ran = true
	})
	dirLib.EndCompileUnit()
	require.Empty(t, pkgLib.CurrentCompileUnit())
	require.True(t, app.RunDeferredBuildsForUnitsOfKind([]string{"dir:foo/bar"}, DeferredBuildKindBinding, nil, nil))
	require.True(t, ran, "deferred task on sibling library must match the active unit")
	require.NoError(t, app.Cache.SaveToDatabase())
}
