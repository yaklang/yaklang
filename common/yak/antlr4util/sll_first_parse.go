package antlr4util

import (
	"os"
	"strings"
	"sync"
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
	sllFirstConfigOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(envAntlrSLLFirst))
		if raw == "" {
			sllFirstEnabledCached = true
			return
		}
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			sllFirstEnabledCached = false
		default:
			sllFirstEnabledCached = true
		}
	})
	return sllFirstEnabledCached
}

func SLLFirstStatsEnabled() bool {
	sllFirstStatsOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(envAntlrSLLFirstStats))
		if raw == "" {
			sllFirstStatsEnabledCached = false
			return
		}
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "y", "on", "enable", "enabled":
			sllFirstStatsEnabledCached = true
		default:
			sllFirstStatsEnabledCached = false
		}
	})
	return sllFirstStatsEnabledCached
}

type SLLFirstCounters struct {
	LLOnly            uint64
	SLLAttempts       uint64
	Fallbacks         uint64
	FallbackCancelled uint64
	FallbackError     uint64
}

var (
	sllFirstConfigOnce    sync.Once
	sllFirstEnabledCached bool

	sllFirstStatsOnce          sync.Once
	sllFirstStatsEnabledCached bool

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
//
// LL fallback is triggered for either ParseCancellationException or a listener
// error collected during the SLL pass. Retrying on listener errors keeps the
// previous behavior of returning the richer LL diagnostics/recovery result.
func ParseASTWithSLLFirst[L antlr.Lexer, P antlr.Parser, T any](
	src string,
	newLexer func(antlr.CharStream) L,
	newParser func(antlr.TokenStream) P,
	decorateTokenSource func(antlr.TokenSource) antlr.TokenSource,
	setup func(lexer L, parser P),
	entry func(parser P) T,
) (T, error) {
	run := func(predictionMode int, errHandler antlr.ErrorStrategy) (ast T, parseErr error, cancelled bool) {
		errListener := NewErrorListener()
		lexer := newLexer(antlr.NewInputStream(src))
		lexer.RemoveErrorListeners()
		lexer.AddErrorListener(errListener)

		tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		if decorateTokenSource != nil {
			tokenStream.SetTokenSource(decorateTokenSource(lexer))
		}
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

	statsEnabled := SLLFirstStatsEnabled()
	if !SLLFirstEnabled() {
		if statsEnabled {
			atomic.AddUint64(&sllFirstLLOnly, 1)
		}
		ast, err, _ := run(antlr.PredictionModeLL, antlr.NewDefaultErrorStrategy())
		return ast, err
	}

	if statsEnabled {
		atomic.AddUint64(&sllFirstSLLAttempts, 1)
	}
	ast, err, cancelled := run(antlr.PredictionModeSLL, antlr.NewBailErrorStrategy())
	if !cancelled && err == nil {
		return ast, nil
	}

	if statsEnabled {
		atomic.AddUint64(&sllFirstFallbacks, 1)
	}
	if cancelled {
		if statsEnabled {
			atomic.AddUint64(&sllFirstFallbackCancelled, 1)
		}
	} else if err != nil {
		if statsEnabled {
			atomic.AddUint64(&sllFirstFallbackError, 1)
		}
	}

	ast, err, _ = run(antlr.PredictionModeLL, antlr.NewDefaultErrorStrategy())
	return ast, err
}
