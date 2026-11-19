package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"

func loadSecurityPrompts() AgentPromptSet {
	return AgentPromptSet{
		textHighStatic:            promptloader.MustLoad("agents/security/high_static/textstream.txt"),
		functionHighStatic:        promptloader.MustLoad("agents/security/high_static/functioncall.txt"),
		DefaultInstruction:        promptloader.MustLoad("agents/security/default_loop/instruction.txt"),
		DefaultOutputExample:      promptloader.MustLoad("agents/security/default_loop/output_example.txt"),
		VerificationInstruction:   promptloader.MustLoad("agents/security/verification/instruction.txt"),
		VerificationOutputExample: promptloader.MustLoad("agents/security/verification/output_example.txt"),
		WorkspaceTemplate:         promptloader.MustLoad("mainloop/workspace.txt"),
		PlanReviewInstruction:     promptloader.MustLoad("agents/security/plan_review/instruction.txt"),
		PlanReviewOutputExample:   promptloader.MustLoad("agents/security/plan_review/output_example.txt"),
		TaskReviewInstruction:     promptloader.MustLoad("agents/security/task_review/instruction.txt"),
		TaskReviewOutputExample:   promptloader.MustLoad("agents/security/task_review/output_example.txt"),
	}
}
