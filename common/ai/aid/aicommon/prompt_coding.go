package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"

func loadCodingPrompts() AgentPromptSet {
	// 两种协议复用同一份固定规则；只在包初始化时拼装，不包含会话数据。
	staticRules := map[string]string{
		"CodingMethodology": promptloader.MustLoad("agents/coding/high_static/methodology.txt"),
		"CodingSystems":     promptloader.MustLoad("agents/coding/high_static/systems.txt"),
	}
	textHighStatic, err := RenderPromptTemplate("coding-textstream-high-static",
		promptloader.MustLoad("agents/coding/high_static/textstream.txt"), staticRules)
	if err != nil {
		panic(err)
	}
	functionHighStatic, err := RenderPromptTemplate("coding-functioncall-high-static",
		promptloader.MustLoad("agents/coding/high_static/functioncall.txt"), staticRules)
	if err != nil {
		panic(err)
	}
	return AgentPromptSet{
		textHighStatic:            textHighStatic,
		functionHighStatic:        functionHighStatic,
		DefaultInstruction:        promptloader.MustLoad("agents/coding/default_loop/instruction.txt"),
		DefaultOutputExample:      promptloader.MustLoad("agents/coding/default_loop/output_example.txt"),
		VerificationInstruction:   promptloader.MustLoad("agents/coding/verification/instruction.txt"),
		VerificationOutputExample: promptloader.MustLoad("agents/coding/verification/output_example.txt"),
		WorkspaceTemplate:         promptloader.MustLoad("agents/coding/workspace_context.txt"),
		PlanReviewInstruction:     promptloader.MustLoad("agents/coding/plan_review/instruction.txt"),
		PlanReviewOutputExample:   promptloader.MustLoad("agents/coding/plan_review/output_example.txt"),
		TaskReviewInstruction:     promptloader.MustLoad("agents/coding/task_review/instruction.txt"),
		TaskReviewOutputExample:   promptloader.MustLoad("agents/coding/task_review/output_example.txt"),
	}
}
