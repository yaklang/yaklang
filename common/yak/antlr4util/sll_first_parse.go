package antlr4util

import (
	"os"
	"strings"
	"sync/atomic"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

const envAntlrSLLFirst = "YAK_ANTLR_SLL_FIRST"
const envAntlrSLLFirstStats = "YAK_ANTLR_SLL_FIRST_STATS"

// SLLFirstEnabled controls whether ANTLR parsing should try SLL mode first and
// fallback to LL mode on parse cancellation.
//
// Default: enabled (for performance). Set YAK_ANTLR_SLL_FIRST=0/false/off to disable.
func SLLFirstEnabled() bool {
	raw := strings.TrimSpace(os.Getenv(envAntlrSLLFirst))
	if raw == "" {
		return true
	}
	switch strings.ToLower(raw) {
	case "0", "false", "no", "off", "disable", "disabled":
		return false
	default:
		return true
	}
}

func SLLFirstStatsEnabled() bool {
	raw := strings.TrimSpace(os.Getenv(envAntlrSLLFirstStats))
	if raw == "" {
		return false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on", "enable", "enabled":
		return true
	default:
		return false
	}
}

type SLLFirstCounters struct {
	LLOnly            uint64
	SLLAttempts       uint64
	Fallbacks         uint64
	FallbackCancelled uint64
	FallbackError     uint64
}

var (
	sllFirstLLOnly            uint64
	sllFirstSLLAttempts       uint64
	sllFirstFallbacks         uint64
	sllFirstFallbackCancelled uint64
	sllFirstFallbackError     uint64
)

func ResetSLLFirstCounters() {
	atomic.StoreUint64(&sllFirstLLOnly, 0)
	atomic.StoreUint64(&sllFirstSLLAttempts, 0)
	atomic.StoreUint64(&sllFirstFallbacks, 0)
	atomic.StoreUint64(&sllFirstFallbackCancelled, 0)
	atomic.StoreUint64(&sllFirstFallbackError, 0)
}

func SLLFirstCountersSnapshot() SLLFirstCounters {
	return SLLFirstCounters{
		LLOnly:            atomic.LoadUint64(&sllFirstLLOnly),
		SLLAttempts:       atomic.LoadUint64(&sllFirstSLLAttempts),
		Fallbacks:         atomic.LoadUint64(&sllFirstFallbacks),
		FallbackCancelled: atomic.LoadUint64(&sllFirstFallbackCancelled),
		FallbackError:     atomic.LoadUint64(&sllFirstFallbackError),
	}
}

// ParseASTWithSLLFirst runs a classic two-stage ANTLR parse:
//  1. Try SLL + BailErrorStrategy (fast, low alloc, no recovery)
//  2. If cancelled, retry LL + DefaultErrorStrategy (correctness + recovery)
//
// It also:
//   - Attaches the yak ErrorListener to both lexer and parser
//   - Detaches lexer tokenSource from tokens after parse to reduce retention
//
// setup is optional and can be used to apply per-language settings such as ANTLR caches.
func ParseASTWithSLLFirst[L antlr.Lexer, P antlr.Parser, T any](
	src string,
	newLexer func(antlr.CharStream) L,
	newParser func(antlr.TokenStream) P,
	setup func(lexer L, parser P),
	entry func(parser P) T,
) (T, error) {
	run := func(predictionMode int, errHandler antlr.ErrorStrategy) (ast T, parseErr error, cancelled bool) {
		errListener := NewErrorListener()
		lexer := newLexer(antlr.NewInputStream(src))
		lexer.RemoveErrorListeners()
		lexer.AddErrorListener(errListener)

		tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		parser := newParser(tokenStream)
		if setup != nil {
			setup(lexer, parser)
		}
		if interpreter := parser.GetInterpreter(); interpreter != nil {
			interpreter.SetPredictionMode(predictionMode)
		}
		parser.RemoveErrorListeners()
		parser.AddErrorListener(errListener)
		parser.SetErrorHandler(errHandler)

		func() {
			defer func() {
				if r := recover(); r != nil {
					if _, ok := r.(*antlr.ParseCancellationException); ok {
						cancelled = true
						return
					}
					panic(r)
				}
			}()
			ast = entry(parser)
		}()

		DetachParserATNSimulatorCaches(parser)
		DetachLexerTokenSource(lexer)
		return ast, errListener.Error(), cancelled
	}

	if !SLLFirstEnabled() {
		atomic.AddUint64(&sllFirstLLOnly, 1)
		ast, err, _ := run(antlr.PredictionModeLL, antlr.NewDefaultErrorStrategy())
		return ast, err
	}

	atomic.AddUint64(&sllFirstSLLAttempts, 1)
	ast, err, cancelled := run(antlr.PredictionModeSLL, antlr.NewBailErrorStrategy())
	if !cancelled && err == nil {
		return ast, nil
	}

	atomic.AddUint64(&sllFirstFallbacks, 1)
	if cancelled {
		atomic.AddUint64(&sllFirstFallbackCancelled, 1)
	} else if err != nil {
		atomic.AddUint64(&sllFirstFallbackError, 1)
	}

	ast, err, _ = run(antlr.PredictionModeLL, antlr.NewDefaultErrorStrategy())
	return ast, err
}
