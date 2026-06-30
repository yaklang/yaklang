package antlr4util

import "github.com/yaklang/antlr/v4"

// BailErrorStrategy is used by the SLL-first parse path to abort immediately on the first
// syntax error instead of running ANTLR's expensive error recovery.
//
// antlr4-go v4.13.1's built-in antlr.NewBailErrorStrategy only calls recognizer.SetError
// (see antlr/antlr4 c0f6ece "eliminate panic/recover as flow control"). Our two-stage
// orchestration in ParseASTWithSLLFirst relies on panic(ParseCancellationException) to bail
// out of the SLL pass and retry in LL, matching the effective behavior of ANTLR 4.11.1
// without vendoring or patching the runtime.
type BailErrorStrategy struct {
	*antlr.DefaultErrorStrategy
}

var _ antlr.ErrorStrategy = &BailErrorStrategy{}

func NewBailErrorStrategy() *BailErrorStrategy {
	return &BailErrorStrategy{
		DefaultErrorStrategy: antlr.NewDefaultErrorStrategy(),
	}
}

func (b *BailErrorStrategy) Recover(recognizer antlr.Parser, e antlr.RecognitionException) {
	context := recognizer.GetParserRuleContext()
	for context != nil {
		context.SetException(e)
		if parent, ok := context.GetParent().(antlr.ParserRuleContext); ok {
			context = parent
		} else {
			context = nil
		}
	}
	panic(antlr.NewParseCancellationException())
}

func (b *BailErrorStrategy) RecoverInline(recognizer antlr.Parser) antlr.Token {
	b.Recover(recognizer, antlr.NewInputMisMatchException(recognizer))
	return nil
}

func (b *BailErrorStrategy) Sync(_ antlr.Parser) {}
