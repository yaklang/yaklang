package javaparser

import (
	"sync"
	"testing"
)

// TestSerializedATNGettersKeepStaticStateLocked guards the sync.Once invariant:
// the exported ATN getters must never rebuild the package-level static state,
// otherwise shared DFA / PredictionContextCache access loses its atn.stateMu
// serialization across concurrently created parsers.
func TestSerializedATNGettersKeepStaticStateLocked(t *testing.T) {
	JavaParserInit()
	JavaLexerInit()

	pAtn := JavaParserParserStaticData.atn
	pDfa := JavaParserParserStaticData.decisionToDFA
	pCache := JavaParserParserStaticData.PredictionContextCache
	lAtn := JavaLexerLexerStaticData.atn
	lCache := JavaLexerLexerStaticData.PredictionContextCache

	if pAtn == nil || pDfa == nil || pCache == nil {
		t.Fatal("parser static state not initialized")
	}
	if lAtn == nil || lCache == nil {
		t.Fatal("lexer static state not initialized")
	}

	for i := 0; i < 8; i++ {
		if len(GetJavaParserSerializedATN()) == 0 {
			t.Fatal("empty parser serialized ATN")
		}
		if len(GetJavaLexerSerializedATN()) == 0 {
			t.Fatal("empty lexer serialized ATN")
		}
	}

	if got := JavaParserParserStaticData.atn; got != pAtn {
		t.Fatal("GetJavaParserSerializedATN replaced the static parser ATN")
	}
	if got := JavaParserParserStaticData.decisionToDFA; &got[0] != &pDfa[0] {
		t.Fatal("GetJavaParserSerializedATN replaced the static parser DFAs")
	}
	if got := JavaParserParserStaticData.PredictionContextCache; got != pCache {
		t.Fatal("GetJavaParserSerializedATN replaced the static parser PredictionContextCache")
	}
	if got := JavaLexerLexerStaticData.atn; got != lAtn {
		t.Fatal("GetJavaLexerSerializedATN replaced the static lexer ATN")
	}
	if got := JavaLexerLexerStaticData.PredictionContextCache; got != lCache {
		t.Fatal("GetJavaLexerSerializedATN replaced the static lexer PredictionContextCache")
	}
}

// TestConcurrentInitAndGettersAreSafe hammers the invariant from many
// goroutines; run with -race it fails if init bypasses sync.Once again.
func TestConcurrentInitAndGettersAreSafe(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = GetJavaParserSerializedATN()
				_ = GetJavaLexerSerializedATN()
				JavaParserInit()
				JavaLexerInit()
			}
		}()
	}
	wg.Wait()
}
