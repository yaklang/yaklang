package aireact

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

func getLoopSchema(disallowAskForClarification bool, disallowPlanAndExecution bool) string {
	var opts []any
	mode := []any{
		ActionDirectlyAnswer, ActionRequireTool,
	}
	if !disallowPlanAndExecution {
		mode = append(mode, ActionRequestPlanExecution)
	}
	if !disallowAskForClarification {
		mode = append(mode, ActionAskForClarification)
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

	opts = append(opts, aitool.WithStructParam(
		"next_action",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Contains the specific action the AI has decided to take. You must choose one action type and provide its corresponding payload."),
		},
		actionFields...,
	), aitool.WithStringParam(
		"cumulative_summary",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("An evolving summary of the conversation. Update this field to include key information from the current interaction that should be remembered for future responses. Include topics discussed, user preferences, important context, and relevant details. If this is the first interaction, create a new summary. If there's existing context, build upon it."),
	), aitool.WithStringParam(
		"human_readable_thought",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. Examples: 'Okay, I understand. Searching for the requested information now...', 'I need to use a tool to get the current stock price.', 'This is a complex request, I will try to execute tool step by step.'"),
	))
	return aitool.NewObjectSchemaWithAction(opts...)
}
