package jspparser

import (
	"sync"

	"github.com/yaklang/antlr/v4"
)

// The generated lexer and parser keep their DFA tables and prediction context
// caches in package-level static data shared by every instance. Prediction only
// ever adds states to those tables; nothing removes them, and
// antlr4util.DetachParserATNSimulatorCaches cannot either, because the tables
// outlive the parser instance. Parsing thousands of JSP files in one process
// therefore grows the tables without bound.
//
// jspRuntimeMu lets one parse hold the tables steady (shared read lock) while a
// periodic reset rebuilds them exclusively. Reset must run between parses.
var jspRuntimeMu sync.RWMutex

// WithJSPRuntimeLock runs fn while the shared DFA tables cannot be reset.
func WithJSPRuntimeLock(fn func() (IJspDocumentsContext, error)) (IJspDocumentsContext, error) {
	jspRuntimeMu.RLock()
	defer jspRuntimeMu.RUnlock()
	return fn()
}

// JSPRuntimeDFAStates reports how many DFA states the shared lexer and parser
// tables currently retain. A fresh table is zero; prediction only adds states.
func JSPRuntimeDFAStates() int {
	jspRuntimeMu.RLock()
	defer jspRuntimeMu.RUnlock()
	return dfaStateCount(JSPLexerLexerStaticData.decisionToDFA) + dfaStateCount(JSPParserParserStaticData.decisionToDFA)
}

func dfaStateCount(dfas []*antlr.DFA) int {
	total := 0
	for _, dfa := range dfas {
		if dfa == nil {
			continue
		}
		total += dfa.Len()
	}
	return total
}

// ResetJSPRuntimeCaches rebuilds the shared DFA tables and prediction context
// caches. It is a no-op for tables that were never initialized.
func ResetJSPRuntimeCaches() {
	jspRuntimeMu.Lock()
	defer jspRuntimeMu.Unlock()

	reset := func(dfa *[]*antlr.DFA, cache **antlr.PredictionContextCache, atn *antlr.ATN) {
		if atn == nil {
			return
		}
		rebuilt := make([]*antlr.DFA, len(atn.DecisionToState))
		for index, state := range atn.DecisionToState {
			rebuilt[index] = antlr.NewDFA(state, index)
		}
		*dfa = rebuilt
		*cache = antlr.NewPredictionContextCache()
	}

	reset(
		&JSPParserParserStaticData.decisionToDFA,
		&JSPParserParserStaticData.PredictionContextCache,
		JSPParserParserStaticData.atn,
	)
	reset(
		&JSPLexerLexerStaticData.decisionToDFA,
		&JSPLexerLexerStaticData.PredictionContextCache,
		JSPLexerLexerStaticData.atn,
	)
}
