package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"

var (
	SharedPlanAndExecHighStaticTemplate             = promptloader.MustLoad("mainloop/textstream/high_static_section.txt")
	SharedPlanAndExecHighStaticFunctionCallTemplate = promptloader.MustLoad("mainloop/functioncall/high_static_section.txt")
	SharedFrozenBlockTemplate                       = promptloader.MustLoad("mainloop/textstream/frozen_block_section.txt")
	SharedFrozenBlockFunctionCallTemplate           = promptloader.MustLoad("mainloop/functioncall/frozen_block_section.txt")
	SharedTaskInstructionSchemaExampleTemplate      = promptloader.MustLoad("mainloop/textstream/semi_dynamic_2_section.txt")
	SharedTaskInstructionFunctionCallTemplate       = promptloader.MustLoad("mainloop/functioncall/semi_dynamic_2_section.txt")
	SharedSemiDynamic1Template                      = promptloader.MustLoad("mainloop/semi_dynamic_1_section.txt")
	SharedTimelineOpenTemplate                      = promptloader.MustLoad("mainloop/timeline_open_section.txt")
)

func MainloopHighStaticTemplate(functionCallMode bool) string {
	if functionCallMode {
		return SharedPlanAndExecHighStaticFunctionCallTemplate
	}
	return SharedPlanAndExecHighStaticTemplate
}

func MainloopFrozenBlockTemplate(functionCallMode bool) string {
	if functionCallMode {
		return SharedFrozenBlockFunctionCallTemplate
	}
	return SharedFrozenBlockTemplate
}

func MainloopSemiDynamic2Template(functionCallMode bool) string {
	if functionCallMode {
		return SharedTaskInstructionFunctionCallTemplate
	}
	return SharedTaskInstructionSchemaExampleTemplate
}
