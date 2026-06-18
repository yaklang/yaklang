package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
)

func init() {
	// Register NewReAct as ReAct-based AIEngineOperator factory function
	// ReAct implements aicommon.AIEngineOperator interface
	aicommon.RegisterReActAIEngineOperator(func(opts ...aicommon.ConfigOption) (aicommon.AIEngineOperator, error) {
		return NewReAct(opts...)
	})

	// Register WithBuiltinTools option
	aicommon.RegisterBuiltinToolsOption(WithBuiltinTools())

	aicommon.RegisterDefaultAIForgeFactoryProvider(func() aicommon.AIForgeFactory {
		return aiforge.NewForgeFactory()
	})
}
