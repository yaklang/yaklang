package ssaapi

import (
	"sync"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/diagnostics"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type fakeAntlrAnalyzer struct {
	cache *ssa.AntlrCache
}

func (f *fakeAntlrAnalyzer) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem {
	return fs
}

func (f *fakeAntlrAnalyzer) InitHandler(*ssa.FunctionBuilder) {}

func (f *fakeAntlrAnalyzer) FilterPreHandlerFile(string) bool {
	return false
}

func (f *fakeAntlrAnalyzer) FilterParseAST(string) bool {
	return true
}

func (f *fakeAntlrAnalyzer) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return src, nil
}

func (f *fakeAntlrAnalyzer) GetAntlrCache() *ssa.AntlrCache {
	return f.cache
}

func (f *fakeAntlrAnalyzer) PreHandlerProject(fi.FileSystem, ssa.FrontAST, *ssa.FunctionBuilder, *memedit.MemEditor) error {
	return nil
}

func (f *fakeAntlrAnalyzer) PreHandlerFile(ssa.FrontAST, *memedit.MemEditor, *ssa.FunctionBuilder) {}

func (f *fakeAntlrAnalyzer) AfterPreHandlerProject(*ssa.FunctionBuilder) {}

func (f *fakeAntlrAnalyzer) UsesDeferredFileBuild() bool {
	return false
}

func (f *fakeAntlrAnalyzer) Clearup() {}

func resetAntlrCacheResetConfigForTest() {
	antlrCacheResetEveryFilesOnce = sync.Once{}
	antlrCacheResetEveryFilesCached = 0
	antlrCacheResetEveryBytesOnce = sync.Once{}
	antlrCacheResetEveryBytesCached = 0
}

func newTestAntlrCache() *ssa.AntlrCache {
	return &ssa.AntlrCache{
		ParserATN: &antlr.ATN{
			DecisionToState: []antlr.DecisionState{antlr.NewBaseDecisionState()},
		},
		ParserDfaCache:               []*antlr.DFA{antlr.NewDFA(antlr.NewBaseDecisionState(), 0)},
		ParserPredictionContextCache: antlr.NewPredictionContextCache(),
	}
}

func TestAntlrCacheResetDefaults(t *testing.T) {
	t.Cleanup(resetAntlrCacheResetConfigForTest)
	resetAntlrCacheResetConfigForTest()

	require.Equal(t, 25, antlrCacheResetEveryFiles())
	require.Equal(t, int64(8*1024*1024), antlrCacheResetEveryBytes())
}

func TestAntlrCacheResetEnvParsing(t *testing.T) {
	t.Cleanup(resetAntlrCacheResetConfigForTest)
	resetAntlrCacheResetConfigForTest()
	t.Setenv("YAK_ANTLR_CACHE_RESET_FILES", "7")
	t.Setenv("YAK_ANTLR_CACHE_RESET_BYTES", "2MB")

	require.Equal(t, 7, antlrCacheResetEveryFiles())
	require.Equal(t, int64(2*1024*1024), antlrCacheResetEveryBytes())

	resetAntlrCacheResetConfigForTest()
	t.Setenv("YAK_ANTLR_CACHE_RESET_FILES", "0")
	t.Setenv("YAK_ANTLR_CACHE_RESET_BYTES", "off")
	require.Equal(t, 0, antlrCacheResetEveryFiles())
	require.Equal(t, int64(0), antlrCacheResetEveryBytes())
}

func TestAntlrASTParseWorker_ResetRuntimeCachesByFileCount(t *testing.T) {
	cache := newTestAntlrCache()
	firstDFA := cache.ParserDfaCache[0]
	firstPredictionCache := cache.ParserPredictionContextCache
	parser := &antlrASTParseWorker{
		language:        &fakeAntlrAnalyzer{cache: cache},
		languageName:    "test",
		resetEveryFiles: 2,
		resetEveryBytes: 0,
	}
	store := parser.initWorker()

	_, err := parser.parseFileAST("a.test", "1", store)
	require.NoError(t, err)
	require.Same(t, firstDFA, cache.ParserDfaCache[0])
	require.Same(t, firstPredictionCache, cache.ParserPredictionContextCache)

	_, err = parser.parseFileAST("b.test", "2", store)
	require.NoError(t, err)
	require.NotSame(t, firstDFA, cache.ParserDfaCache[0])
	require.NotSame(t, firstPredictionCache, cache.ParserPredictionContextCache)
}

func TestAntlrASTParseWorker_ResetRuntimeCachesBySourceBytes(t *testing.T) {
	cache := newTestAntlrCache()
	firstDFA := cache.ParserDfaCache[0]
	parser := &antlrASTParseWorker{
		language:        &fakeAntlrAnalyzer{cache: cache},
		languageName:    "test",
		resetEveryFiles: 0,
		resetEveryBytes: 4,
	}
	store := parser.initWorker()

	_, err := parser.parseFileAST("a.test", "12", store)
	require.NoError(t, err)
	require.Same(t, firstDFA, cache.ParserDfaCache[0])

	_, err = parser.parseFileAST("b.test", "34", store)
	require.NoError(t, err)
	require.NotSame(t, firstDFA, cache.ParserDfaCache[0])
}

func newASTWindowTestConfig(t *testing.T, language ssaconfig.Language, concurrency int, projectBytes int64, diag bool) *Config {
	t.Helper()

	cfg, err := ssaconfig.NewCLIScanConfig(
		ssaconfig.WithProjectLanguage(language),
		ssaconfig.WithCompileConcurrency(concurrency),
		ssaconfig.WithCompileDiagnostics(diag),
	)
	require.NoError(t, err)
	cfg.SetCompileProjectBytes(projectBytes)
	return &Config{Config: cfg}
}

func TestResolveASTBuildWindow_PHPTraceKeepsBalancedWindow(t *testing.T) {
	oldLevel := diagnostics.GetLevel()
	diagnostics.SetLevel(diagnostics.LevelLow)
	t.Cleanup(func() {
		diagnostics.SetLevel(oldLevel)
	})
	t.Setenv("YAK_SSA_AST_MEMORY_BUDGET", "10GiB")

	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, true)
	decision := cfg.resolveASTBuildWindow(cfg.GetCompileConcurrency())

	require.True(t, decision.largeProject)
	require.True(t, decision.diagnosticsHeavy)
	require.Equal(t, 2, decision.window)
	require.Equal(t, int64(10*1024*1024*1024), decision.budgetBytes)
	require.Equal(t, int64(4*1024*1024*1024), decision.slotCostBytes)
}

func TestResolveASTBuildWindow_PHPWithoutTraceAllowsMoreCPU(t *testing.T) {
	oldLevel := diagnostics.GetLevel()
	diagnostics.SetLevel(diagnostics.LevelNormal)
	t.Cleanup(func() {
		diagnostics.SetLevel(oldLevel)
	})
	t.Setenv("YAK_SSA_AST_MEMORY_BUDGET", "10GiB")

	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, false)
	decision := cfg.resolveASTBuildWindow(cfg.GetCompileConcurrency())

	require.True(t, decision.largeProject)
	require.False(t, decision.diagnosticsHeavy)
	require.Equal(t, 2, decision.window)
	require.Equal(t, int64(4*1024*1024*1024), decision.slotCostBytes)
}

func TestResolveASTBuildWindow_PHPLowBudgetFallsBackToOne(t *testing.T) {
	t.Setenv("YAK_SSA_AST_MEMORY_BUDGET", "6GiB")

	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, true)
	decision := cfg.resolveASTBuildWindow(cfg.GetCompileConcurrency())

	require.True(t, decision.largeProject)
	require.Equal(t, 1, decision.window)
	require.Equal(t, int64(4*1024*1024*1024), decision.slotCostBytes)
}

func TestResolveASTBuildWindow_ManualWindowEnvWins(t *testing.T) {
	t.Setenv("YAK_SSA_AST_MEMORY_BUDGET", "10GiB")
	t.Setenv("YAK_SSA_AST_BUILD_WINDOW_FILES", "6")

	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, true)
	decision := cfg.resolveASTBuildWindow(cfg.GetCompileConcurrency())

	require.True(t, decision.largeProject)
	require.True(t, decision.manualOverride)
	require.Zero(t, decision.window)
}

func TestResolveASTBuildWindow_SkipsSmallProjects(t *testing.T) {
	t.Setenv("YAK_SSA_AST_MEMORY_BUDGET", "10GiB")

	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 4*1024*1024, true)
	decision := cfg.resolveASTBuildWindow(cfg.GetCompileConcurrency())

	require.False(t, decision.largeProject)
	require.Zero(t, decision.window)
}

func TestResolveLargeProjectGCPercent_AutoForLargePreHandlerProject(t *testing.T) {
	t.Setenv("GOGC", "")
	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, true)

	decision := cfg.resolveLargeProjectGCPercent()

	require.True(t, decision.largeProject)
	require.False(t, decision.manualOverride)
	require.Equal(t, defaultLargeProjectGC, decision.percent)
	require.Equal(t, "auto:large-project", decision.source)
}

func TestResolveLargeProjectGCPercent_RespectsGOGC(t *testing.T) {
	t.Setenv("GOGC", "50")
	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, true)

	decision := cfg.resolveLargeProjectGCPercent()

	require.True(t, decision.largeProject)
	require.True(t, decision.manualOverride)
	require.Zero(t, decision.percent)
	require.Equal(t, "env:GOGC", decision.source)
}

func TestResolveLargeProjectGCPercent_EnvOverride(t *testing.T) {
	t.Setenv("GOGC", "")
	t.Setenv("YAK_SSA_GC_PERCENT", "150")
	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, true)

	decision := cfg.resolveLargeProjectGCPercent()

	require.True(t, decision.largeProject)
	require.False(t, decision.manualOverride)
	require.Equal(t, 150, decision.percent)
	require.Equal(t, "env:YAK_SSA_GC_PERCENT", decision.source)
}

func TestResolveLargeProjectGCPercent_DisabledByEnv(t *testing.T) {
	t.Setenv("GOGC", "")
	t.Setenv("YAK_SSA_GC_PERCENT", "off")
	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 60*1024*1024, true)

	decision := cfg.resolveLargeProjectGCPercent()

	require.True(t, decision.largeProject)
	require.True(t, decision.manualOverride)
	require.Zero(t, decision.percent)
	require.Equal(t, "env:YAK_SSA_GC_PERCENT", decision.source)
}

func TestResolveLargeProjectGCPercent_SkipsSmallProjects(t *testing.T) {
	t.Setenv("GOGC", "")
	cfg := newASTWindowTestConfig(t, ssaconfig.PHP, 12, 4*1024*1024, true)

	decision := cfg.resolveLargeProjectGCPercent()

	require.False(t, decision.largeProject)
	require.Zero(t, decision.percent)
}
