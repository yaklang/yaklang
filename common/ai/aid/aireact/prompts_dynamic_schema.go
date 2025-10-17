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
	var opts []any

	actionFields := []aitool.ToolOption{
		aitool.WithStringParam(
			"type",
			aitool.WithParam_Description("You MUST use 'directly_answer' as the action type."),
			aitool.WithParam_Enum(ActionDirectlyAnswer),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("answer_payload", aitool.WithParam_Description("Provide the final, complete answer for the user here. The content should be self-contained and ready to be displayed."), aitool.WithParam_Required(true)),
	}

	opts = append(opts, aitool.WithStructParam(
		"next_action",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Contains the direct answer action."),
		},
		actionFields...,
	), aitool.WithStringParam(
		"cumulative_summary",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("An evolving summary of the conversation. Update this field to include key information from the current interaction that should be remembered for future responses. Include topics discussed, user preferences, important context, and relevant details. If this is the first interaction, create a new summary. If there's existing context, build upon it."),
	))
	return aitool.NewObjectSchemaWithAction(opts...)
}
