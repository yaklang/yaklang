package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func init() {
	// Register NewReAct as ReAct-based AIEngineOperator factory function
	// ReAct implements aicommon.AIEngineOperator interface
	aicommon.RegisterReActAIEngineOperator(func(opts ...aicommon.ConfigOption) (aicommon.AIEngineOperator, error) {
		return NewReAct(opts...)
	})

	// Register WithBuiltinTools option
	aicommon.RegisterBuiltinToolsOption(WithBuiltinTools())
}
