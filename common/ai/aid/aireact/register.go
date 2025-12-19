package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func init() {
	// Register NewReAct factory function
	aicommon.RegisterNewReAct(func(opts ...aicommon.ConfigOption) (aicommon.ReActIF, error) {
		return NewReAct(opts...)
	})

	// Register WithBuiltinTools option
	aicommon.RegisterBuiltinToolsOption(WithBuiltinTools())
}
