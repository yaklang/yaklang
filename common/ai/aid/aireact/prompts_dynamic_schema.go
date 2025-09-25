package aireact

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

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

func getLoopSchema(
	disallowAskForClarification,
	disableKnowledgeEnhanceAnswer,
	disallowPlanAndExecution,
	disallowWriteYaklangCode bool,
	haveAIForgeList bool,
) string {
	var opts []any
	mode := []any{
		ActionDirectlyAnswer, ActionRequireTool,
	}
	if !disallowPlanAndExecution {
		mode = append(mode, ActionRequestPlanExecution)
		if haveAIForgeList {
			mode = append(mode, ActionRequireAIBlueprintForge)
		}
	}
	if !disallowAskForClarification {
		mode = append(mode, ActionAskForClarification)
	}

	if !disableKnowledgeEnhanceAnswer {
		mode = append(mode, ActionKnowledgeEnhanceAnswer)
	}

	if !disallowWriteYaklangCode {
		mode = append(mode, ActionWriteYaklangCode)
	}

	actionFields := []aitool.ToolOption{
		aitool.WithStringParam(
			"type",
			aitool.WithParam_Description("You MUST choose one of following action types. The value you select here determines which of the other fields in this 'action' object you should populate."),
			aitool.WithParam_Enum(mode...),
			aitool.WithParam_Required(true),
		),
	}
	actionFields = append(
		actionFields,
		aitool.WithStringParam("answer_payload", aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'directly_answer'. Provide the final, complete answer for the user here. The content should be self-contained and ready to be displayed.")),
		aitool.WithStringParam("tool_require_payload", aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'web_search', 'database_query'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here.")),
		aitool.WithBoolParam(
			"middle_step",
			aitool.WithParam_Description("CRUCIAL for multi-tool tasks. Use ONLY with 'tool_require_payload'. Set to 'true' if this tool call is an intermediate step in a sequence to solve a complex task. Set to 'false' if this is the FINAL tool call needed before you can provide the complete answer. Before starting, you should outline your multi-step plan in 'cumulative_summary'."),
			aitool.WithParam_Required(true),
		),
	)
	if !disallowPlanAndExecution {
		actionFields = append(
			actionFields,
			aitool.WithStringParam(
				"plan_request_payload",
				aitool.WithParam_Description(
					"USE THIS FIELD ONLY IF type is 'request_plan_and_execution'. Provide a one-sentence summary of the complex task that needs a multi-step plan. This summary will trigger a more advanced planning system. Example: 'Create a marketing plan for a new product launch.'",
				),
			),
		)
		if haveAIForgeList {
			aitool.WithStringParam(
				"blueprint_payload",
				aitool.WithParam_Description(
					"USE THIS FIELD ONLY IF type is 'require_ai_blueprint'. Provide a forge blueprint ID from the available AI blueprint list to execute the task.",
				),
			)
		}
	}
	if !disallowAskForClarification {
		actionFields = append(actionFields, aitool.WithStructParam(
			"ask_for_clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Use this action when user's intent is ambiguous or incomplete."),
			},
			aitool.WithStringParam("question", aitool.WithParam_Required(true), aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details.")),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description(
					`Optional additional context that may help the user understand what information is needed. This can include examples or explanations of why the clarification is necessary.`,
				),
			),
		))
	}

	if !disallowWriteYaklangCode {
		actionFields = append(actionFields, aitool.WithStringParam(
			"write_yaklang_code_approach",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'write_yaklang_code' 编写Yaklang代码的具体思路和策略。应该包含：1) 代码的核心逻辑设计思路和主要执行流程；2) 需要使用的Yaklang内置库和关键函数（如poc、synscan、servicescan、crawler、等安全库）；3) 数据处理方式和结果输出策略；4) 并发控制方案（是否使用go关键字、SizedWaitGroup限制并发数等）；5) 错误处理策略（优先使用~操作符进行简洁的错误处理）；6) 用户交互设计（如cli参数接收、进度显示等）。重点阐述如何充分利用Yaklang的安全特性、语法糖和内置能力来高效实现目标功能，确保代码既简洁又强大。"),
		))
	}

	opts = append(opts, aitool.WithStringParam(
		"human_readable_thought",
		aitool.WithParam_Required(false),
		aitool.WithParam_Description("[Not a must-being] Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. Examples: 'Okay, I understand. Searching for the requested information now...', 'I need to use a tool to get the current stock price.', 'This is a complex request, I will try to execute tool step by step.' if direct-answer mode, no need to fill this field"),
	), aitool.WithStructParam(
		"next_action",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Contains the specific action the AI has decided to take. You must choose one action type and provide its corresponding payload."),
		},
		actionFields...,
	), aitool.WithStringParam(
		"cumulative_summary",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("An evolving summary of the conversation. Update this field to include key information from the current interaction that should be remembered for future responses. Include topics discussed, user preferences, important context, and relevant details. If this is the first interaction, create a new summary. If there's existing context, build upon it."),
	))
	return aitool.NewObjectSchemaWithAction(opts...)
}

func getYaklangCodeLoopSchema() string {
	opts := []any{
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Description("You MUST choose one of the following action types for the Yaklang code generation loop. What you choose will determine the next-step behavior in the code generation process.\n"+
				"'write_code'  or 'modify_code' means you have to provide Yaklang Code in <|GEN_CODE_...|>, check the example after schema"),
			aitool.WithParam_EnumString(
				"query_document",
				"write_code",
				"modify_code",
				"require_tool",
				"ask_for_clarification",
			),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"human_readable_thought",
			aitool.WithParam_Description("Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. Examples: 'Okay, I understand. Searching for the requested information now...', 'I need to use a tool to get the current stock price.', 'This is a complex request, I will try to execute tool step by step.'"),
		),
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'check-yaklang-syntax', 'yak-document'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here."),
		),
		aitool.WithNumberParam(
			"modify_start_line",
			aitool.WithParam_Description("If the action is 'modify_code', specify the starting line number (1-based index) of the code segment to be modified. This indicates where the changes should begin in the existing code."),
		),
		aitool.WithNumberParam(
			"modify_end_line",
			aitool.WithParam_Description("If the action is 'modify_code', specify the ending line number (1-based index) of the code segment to be modified. This indicates where the changes should end in the existing code. The lines from modify_start_line to modify_end_line (inclusive) will be replaced with the new code provided in the 'code' field."),
		),
		aitool.WithStringParam(
			"query_document",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'query_document'. Provide the exact name of the document you need to query (e.g., 'yaklang-syntax', 'yaklang-document'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here."),
		),
		aitool.WithStructParam(
			"ask_for_clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Use this action when user's intent is ambiguous or incomplete."),
			},
			aitool.WithStringParam("question", aitool.WithParam_Required(true), aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details.")),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description(
					`Optional additional context that may help the user understand what information is needed. This can include examples or explanations of why the clarification is necessary.`,
				),
			),
		),
	}
	return aitool.NewObjectSchema(opts...)
}
