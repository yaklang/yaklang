package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func getReSelectTool(
	allowAskForClarification bool,
) string {
	var opts []any

	actionEnum := []string{
		"require-tool", "abandon",
	}
	if allowAskForClarification {
		actionEnum = []string{"require-tool", "ask-for-clarification", "abandon"}
	}
	rules := []string{
		"如果你决定使用某个工具，请选择require-tool，设置tool为你选择的工具名称",
		"如果你认为当前任务不需要工具，请选择abandon，设置tool为空字符串",
	}
	if allowAskForClarification {
		rules = append(rules, "如果你不确定用户的意图，请选择ask-for-clarification，设置tool为空字符串")
	}

	opts = append(opts, aitool.WithStringParam(
		"@action",
		aitool.WithParam_Description("You MUST choose one of the following action types."+
			" What you choose will determine the next-step behavior.",
		),
		aitool.WithParam_EnumString(actionEnum...),
		aitool.WithParam_Required(true),
		aitool.WithParam_Raw("x-rules", rules),
	))
	opts = append(opts, aitool.WithStringParam("tool", aitool.WithParam_Description("Your tool that will be used.")))
	opts = append(opts, aitool.WithStringParam("abandon_reason", aitool.WithParam_Description("If you choose 'abandon', please provide a brief reason for abandoning the tool usage.")))

	if allowAskForClarification {
		opts = append(opts, aitool.WithStructParam(
			"clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(false),
			},
			aitool.WithStringParam(
				"question",
				aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details."), aitool.WithParam_Required(true),
			),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description("A list of options for helping user option or suggestions. This can include examples or explanations of why the clarification is necessary."),
			),
		))
	}
	return aitool.NewObjectSchema(opts...)
}

func getDirectlyAnswer() string {
	return aitool.NewObjectSchemaWithActionName(
		"directly_answer",
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(
				`USE THIS FIELD ONLY IF @action is 'directly_answer' AND answer is short (≤200 chars). For long answers, leave this empty and use '<|FINAL_ANSWER_...|>' tags after JSON. ⚠️ CRITICAL: answer_payload and <|FINAL_ANSWER_...|> are STRICTLY MUTUALLY EXCLUSIVE - never use both simultaneously.`,
			),
		),
	)
}
