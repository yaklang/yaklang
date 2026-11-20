package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"

// PromptProtocol selects the output contract used by prompt templates.
// The zero value retains the default text-stream protocol.
type PromptProtocol uint8

const (
	PromptProtocolTextStream PromptProtocol = iota
	PromptProtocolFunctionCall
)

// Shared section wrappers depend on the output protocol, not the agent mode.
var (
	SharedFrozenBlockTemplate                  = promptloader.MustLoad("mainloop/textstream/frozen_block_section.txt")
	SharedFrozenBlockFunctionCallTemplate      = promptloader.MustLoad("mainloop/functioncall/frozen_block_section.txt")
	SharedTaskInstructionSchemaExampleTemplate = promptloader.MustLoad("mainloop/textstream/semi_dynamic_2_section.txt")
	SharedTaskInstructionFunctionCallTemplate  = promptloader.MustLoad("mainloop/functioncall/semi_dynamic_2_section.txt")
	SharedSemiDynamic1Template                 = promptloader.MustLoad("mainloop/semi_dynamic_1_section.txt")
	SharedTimelineOpenTemplate                 = promptloader.MustLoad("mainloop/timeline_open_section.txt")
)

func MainloopFrozenBlockTemplate(protocol PromptProtocol) string {
	if protocol == PromptProtocolFunctionCall {
		return SharedFrozenBlockFunctionCallTemplate
	}
	return SharedFrozenBlockTemplate
}

func MainloopSemiDynamic2Template(protocol PromptProtocol) string {
	if protocol == PromptProtocolFunctionCall {
		return SharedTaskInstructionFunctionCallTemplate
	}
	return SharedTaskInstructionSchemaExampleTemplate
}
