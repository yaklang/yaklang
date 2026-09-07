package pythonparser

import (
	"sync"
	"testing"
)

// TestSerializedATNGettersKeepStaticStateLocked guards the sync.Once invariant:
// the exported ATN getters must never rebuild the package-level static state.
// If they do, parsers created concurrently can end up sharing a
// PredictionContextCache while locking different ATN mutexes, which aborts the
// process with "fatal error: concurrent map read and map write".
func TestSerializedATNGettersKeepStaticStateLocked(t *testing.T) {
	PythonParserInit()
	PythonLexerInit()

	pAtn := PythonParserParserStaticData.atn
	pDfa := PythonParserParserStaticData.decisionToDFA
	pCache := PythonParserParserStaticData.PredictionContextCache
	lAtn := PythonLexerLexerStaticData.atn
	lDfa := PythonLexerLexerStaticData.decisionToDFA
	lCache := PythonLexerLexerStaticData.PredictionContextCache

	if pAtn == nil || pDfa == nil || pCache == nil {
		t.Fatal("parser static state not initialized")
	}
	if lAtn == nil || lDfa == nil || lCache == nil {
		t.Fatal("lexer static state not initialized")
	}

	for i := 0; i < 8; i++ {
		if len(GetPythonParserSerializedATN()) == 0 {
			t.Fatal("empty parser serialized ATN")
		}
		if len(GetPythonLexerSerializedATN()) == 0 {
			t.Fatal("empty lexer serialized ATN")
		}
	}

	if got := PythonParserParserStaticData.atn; got != pAtn {
		t.Fatal("GetPythonParserSerializedATN replaced the static parser ATN")
	}
	if got := PythonParserParserStaticData.decisionToDFA; &got[0] != &pDfa[0] {
		t.Fatal("GetPythonParserSerializedATN replaced the static parser DFAs")
	}
	if got := PythonParserParserStaticData.PredictionContextCache; got != pCache {
		t.Fatal("GetPythonParserSerializedATN replaced the static parser PredictionContextCache")
	}
	if got := PythonLexerLexerStaticData.atn; got != lAtn {
		t.Fatal("GetPythonLexerSerializedATN replaced the static lexer ATN")
	}
	if got := PythonLexerLexerStaticData.PredictionContextCache; got != lCache {
		t.Fatal("GetPythonLexerSerializedATN replaced the static lexer PredictionContextCache")
	}
}

// TestConcurrentInitAndGettersAreSafe hammers the same invariant from many
// goroutines; run with -race it fails if init bypasses sync.Once again.
func TestConcurrentInitAndGettersAreSafe(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = GetPythonParserSerializedATN()
				_ = GetPythonLexerSerializedATN()
				PythonParserInit()
				PythonLexerInit()
			}
		}()
	}
	wg.Wait()
}
