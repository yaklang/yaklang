package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AgentPromptSet owns mode-specific content. Protocol, cache sections and caller
// schemas remain shared. Returned values contain immutable strings only.
type AgentPromptSet struct {
	textHighStatic            string
	functionHighStatic        string
	DefaultInstruction        string
	DefaultOutputExample      string
	VerificationInstruction   string
	VerificationOutputExample string
	WorkspaceTemplate         string
}

func (p AgentPromptSet) HighStatic(functionCallMode bool) string {
	if functionCallMode {
		return p.functionHighStatic
	}
	return p.textHighStatic
}

func loadAgentPrompts(mode string) AgentPromptSet {
	root := "agents/" + mode + "/"
	static := func(protocol string) string {
		content := promptloader.MustLoad(root + "high_static/" + protocol + ".txt")
		if mode == ypb.AgentModeCoding {
			// Compose fixed fragments once, never with session data.
			var err error
			content, err = RenderPromptTemplate("coding-high-static", content, map[string]string{
				"CodingMethodology": promptloader.MustLoad(root + "high_static/methodology.txt"),
				"CodingSystems":     promptloader.MustLoad(root + "high_static/systems.txt"),
			})
			if err != nil {
				panic(err)
			}
		}
		return content
	}
	workspace := "mainloop/workspace.txt"
	if mode == ypb.AgentModeCoding {
		workspace = root + "workspace_context.txt"
	}
	return AgentPromptSet{
		textHighStatic: static("textstream"), functionHighStatic: static("functioncall"),
		DefaultInstruction:        promptloader.MustLoad(root + "default_loop/instruction.txt"),
		DefaultOutputExample:      promptloader.MustLoad(root + "default_loop/output_example.txt"),
		VerificationInstruction:   promptloader.MustLoad(root + "verification/instruction.txt"),
		VerificationOutputExample: promptloader.MustLoad(root + "verification/output_example.txt"),
		WorkspaceTemplate:         promptloader.MustLoad(workspace),
	}
}

var (
	securityPrompts                            = loadAgentPrompts(ypb.AgentModeSecurity)
	codingPrompts                              = loadAgentPrompts(ypb.AgentModeCoding)
	SharedFrozenBlockTemplate                  = promptloader.MustLoad("mainloop/textstream/frozen_block_section.txt")
	SharedFrozenBlockFunctionCallTemplate      = promptloader.MustLoad("mainloop/functioncall/frozen_block_section.txt")
	SharedTaskInstructionSchemaExampleTemplate = promptloader.MustLoad("mainloop/textstream/semi_dynamic_2_section.txt")
	SharedTaskInstructionFunctionCallTemplate  = promptloader.MustLoad("mainloop/functioncall/semi_dynamic_2_section.txt")
	SharedSemiDynamic1Template                 = promptloader.MustLoad("mainloop/semi_dynamic_1_section.txt")
	SharedTimelineOpenTemplate                 = promptloader.MustLoad("mainloop/timeline_open_section.txt")
)

// AgentPrompts is the single content selector. Config/start-parameter validation
// rejects unsupported modes before prompt assembly; an unspecified mode is security.
func AgentPrompts(mode string) AgentPromptSet {
	if mode == ypb.AgentModeCoding {
		return codingPrompts
	}
	return securityPrompts
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
