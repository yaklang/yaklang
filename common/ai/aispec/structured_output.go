package aispec

import (
	"fmt"
	"sync/atomic"
)

// StructuredOutputExecutor lets the gateway call the shared LiteForge runtime
// without importing aicommon, which itself depends on the gateway package.
type StructuredOutputExecutor func(string, map[string]any, GeneralChatter, ...AIConfigOption) (map[string]any, error)

var structuredOutputExecutor atomic.Pointer[StructuredOutputExecutor]

func RegisterStructuredOutputExecutor(execute StructuredOutputExecutor) {
	structuredOutputExecutor.Store(&execute)
}

func ExecuteStructuredOutput(input string, fields map[string]any, chat GeneralChatter, opts ...AIConfigOption) (map[string]any, error) {
	execute := structuredOutputExecutor.Load()
	if execute == nil || *execute == nil {
		return nil, fmt.Errorf("structured output executor is not registered: import common/ai/aid/liteforge")
	}
	return (*execute)(input, fields, chat, opts...)
}
