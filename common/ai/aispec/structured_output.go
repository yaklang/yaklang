package aispec

import (
	"errors"
	"sync/atomic"
)

// StructuredOutputExecutor lets the gateway call the shared LiteForge runtime
// without importing aicommon, which itself depends on the gateway package.
type StructuredOutputExecutor func(string, map[string]any, GeneralChatter, ...AIConfigOption) (map[string]any, error)

var structuredOutputExecutor atomic.Pointer[StructuredOutputExecutor]

var ErrStructuredOutputExecutorNotRegistered = errors.New(`LiteForge structured output executor is not registered. Add the following blank import to your Go entry point to register it:

import (
	_ "github.com/yaklang/yaklang/common/ai/aid/liteforge"
)

Standard Yak entry points load LiteForge automatically; custom Go entry points must import it explicitly.`)

// RequireStructuredOutputExecutor reports missing initialization before model
// selection, so provider fallback cannot hide the actionable import instruction.
func RequireStructuredOutputExecutor() error {
	execute := structuredOutputExecutor.Load()
	if execute == nil || *execute == nil {
		return ErrStructuredOutputExecutorNotRegistered
	}
	return nil
}

func RegisterStructuredOutputExecutor(execute StructuredOutputExecutor) {
	structuredOutputExecutor.Store(&execute)
}

func ExecuteStructuredOutput(input string, fields map[string]any, chat GeneralChatter, opts ...AIConfigOption) (map[string]any, error) {
	execute := structuredOutputExecutor.Load()
	if execute == nil || *execute == nil {
		return nil, ErrStructuredOutputExecutorNotRegistered
	}
	return (*execute)(input, fields, chat, opts...)
}
