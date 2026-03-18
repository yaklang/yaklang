package ssa_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestAntlrCache_ResetRuntimeCaches(t *testing.T) {
	base := ssa.NewPreHandlerBase()
	cache := base.CreateAntlrCache(
		javaparser.GetJavaLexerSerializedATN(),
		javaparser.GetJavaParserSerializedATN(),
	)
	require.NotNil(t, cache)

	require.NotNil(t, cache.ParserATN)
	require.NotNil(t, cache.LexerATN)

	require.NotNil(t, cache.ParserDfaCache)
	require.NotNil(t, cache.LexerDfaCache)

	oldParserCtxCache := cache.ParserPredictionContextCache
	oldLexerCtxCache := cache.LexerPredictionContextCache

	cache.ResetRuntimeCaches()

	require.NotNil(t, cache.ParserPredictionContextCache)
	require.NotNil(t, cache.LexerPredictionContextCache)
	require.NotSame(t, oldParserCtxCache, cache.ParserPredictionContextCache)
	require.NotSame(t, oldLexerCtxCache, cache.LexerPredictionContextCache)

	require.Len(t, cache.ParserDfaCache, len(cache.ParserATN.DecisionToState))
	require.Len(t, cache.LexerDfaCache, len(cache.LexerATN.DecisionToState))
}

func TestAntlrCache_Clear(t *testing.T) {
	base := ssa.NewPreHandlerBase()
	cache := base.CreateAntlrCache(
		javaparser.GetJavaLexerSerializedATN(),
		javaparser.GetJavaParserSerializedATN(),
	)
	require.NotNil(t, cache)

	cache.Clear()

	require.Nil(t, cache.ParserDfaCache)
	require.Nil(t, cache.LexerDfaCache)
	require.Nil(t, cache.ParserPredictionContextCache)
	require.Nil(t, cache.LexerPredictionContextCache)

	// ATN should remain intact so the cache can be reinitialized later.
	require.NotNil(t, cache.ParserATN)
	require.NotNil(t, cache.LexerATN)
}
