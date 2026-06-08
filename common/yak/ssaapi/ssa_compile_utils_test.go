package ssaapi

import (
	"sync"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type fakeAntlrAnalyzer struct {
	cache *ssa.AntlrCache
}

func (f *fakeAntlrAnalyzer) WrapWithPreprocessedFS(fs fi.FileSystem) fi.FileSystem {
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
