package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"

var (
	codingSystemsTemplate                           = promptloader.MustLoad("mainloop/coding/systems.txt")
	codingHighStaticTemplate                        = promptloader.MustLoad("mainloop/coding/textstream/high_static_section.txt")
	codingHighStaticFunctionCallTemplate            = promptloader.MustLoad("mainloop/coding/functioncall/high_static_section.txt")
	codingMethodologyTemplate                       = promptloader.MustLoad("mainloop/coding/methodology.txt")
	SharedPlanAndExecHighStaticTemplate             = promptloader.MustLoad("mainloop/textstream/high_static_section.txt")
	SharedPlanAndExecHighStaticFunctionCallTemplate = promptloader.MustLoad("mainloop/functioncall/high_static_section.txt")
	SharedFrozenBlockTemplate                       = promptloader.MustLoad("mainloop/textstream/frozen_block_section.txt")
	SharedFrozenBlockFunctionCallTemplate           = promptloader.MustLoad("mainloop/functioncall/frozen_block_section.txt")
	SharedTaskInstructionSchemaExampleTemplate      = promptloader.MustLoad("mainloop/textstream/semi_dynamic_2_section.txt")
	SharedTaskInstructionFunctionCallTemplate       = promptloader.MustLoad("mainloop/functioncall/semi_dynamic_2_section.txt")
	SharedSemiDynamic1Template                      = promptloader.MustLoad("mainloop/semi_dynamic_1_section.txt")
	SharedTimelineOpenTemplate                      = promptloader.MustLoad("mainloop/timeline_open_section.txt")
)

// HighStaticData contains only static coding guidance, never paths or live Git state.
func (m *PromptMaterials) HighStaticData() map[string]any {
	return map[string]any{
		"CodingMethodology": codingMethodologyTemplate,
		"CodingSystems":     codingSystemsTemplate,
	}
}

func MainloopHighStaticTemplate(functionCallMode, codingMode bool) string {
	if codingMode {
		if functionCallMode {
			return codingHighStaticFunctionCallTemplate
		}
		return codingHighStaticTemplate
	}
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
