package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type LoopActionFactory func(r aicommon.AIInvokeRuntime) (*LoopAction, error)

type LoopActionVerifierFunc func(loop *ReActLoop, action *aicommon.Action) error
type LoopActionHandlerFunc func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator)

type LoopAction struct {
	// plan 与 forge executor 会允许支持异步执行，异步情况下仍然允许对话和其他功能
	AsyncMode         bool
	ActionType        string `json:"type"`
	Description       string `json:"description"`
	Options           []aitool.ToolOption
	ActionVerifier    LoopActionVerifierFunc
	ActionHandler     LoopActionHandlerFunc
	StreamFields      []*LoopStreamField
	AITagStreamFields []*LoopAITagField

	// OutputExamples provides usage examples for this action, describing when and how to use it.
	// This field helps AI understand the appropriate scenarios for selecting this action.
	OutputExamples string `json:"output_examples,omitempty"`
}

func buildSchema(actions ...*LoopAction) string {
	var actionNames []string
	var actionDesc []string
	for _, action := range actions {
		actionNames = append(actionNames, action.ActionType)

		// Build description with metadata if available
		desc := action.ActionType + ": " + action.Description

		// Check if this is a loop action and has metadata with usage prompt
		if meta, ok := GetLoopMetadata(action.ActionType); ok && meta.UsagePrompt != "" {
			desc = action.ActionType + ": " + meta.UsagePrompt
		}

		actionDesc = append(actionDesc, desc)
	}
	var opts = []any{
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Description("required '@action' field to identify the action type"),
			aitool.WithParam_EnumString(actionNames...),
			aitool.WithParam_Required(true),
			aitool.WithParam_Raw("x-@action-rules", actionDesc),
		),
		aitool.WithStringParam(
			"identifier",
			aitool.WithParam_Description(
				"REQUIRED. A short snake_case label (lowercase + underscores, <=30 chars) describing the PURPOSE of this action call. "+
					"Examples: folder_skeleton, read_go_mod, grep_sql_exec, write_dir_structure. "+
					"This identifier is used in log file paths to help users quickly understand what each action call is doing.",
			),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"human_readable_thought",
			aitool.WithParam_Description(
				"Optional. Omit this field when @action is 'directly_answer' or when the next step is already obvious. If you do provide it, keep it to one short, action-oriented sentence only (prefer <=12 Chinese characters or <=8 English words).",
			),
		),
		todoDeltaSchemaOption(),
	}

	existed := make(map[string]struct{})
	existed["@action"] = struct{}{}
	existed["identifier"] = struct{}{}
	existed["human_readable_thought"] = struct{}{}

	for _, action := range actions {
		if action == nil {
			continue
		}
		if len(action.Options) <= 0 {
			continue
		}
		for _, opt := range action.Options {
			opts = append(opts, opt)
		}
	}

	return aitool.NewObjectSchema(opts...)
}

func todoDeltaSchemaOption() aitool.ToolOption {
	return aitool.WithStructParam("todo_delta", []aitool.PropertyOption{
		aitool.WithParam_Description("The only write channel for the short-term TODO work set; TODO LIST is read-only and prose does not update it. This field is structurally optional but mandatory in the same normal action whenever TODO state is initialized or any item is added, refined, closed, deferred, resumed, or switched; omit it only when the work set truly does not change. For a task with two or more independent steps, targets, or entry points, or any explore-then-verify flow, initialize all known work in the first executable action, set one explicit-id item current, and advance it in that action. Apply order is add, update, close, current. Open TODOs form the Frontier; the unique current item is the primary focus. When an Observation opens one or more qualified branches that need later return, record every qualified branch with its concrete target, triggering evidence, falsifiable hypothesis, and first resume action before advancing current. A branch may execute after current closes or prerequisites are met; its tool need not be exposed this turn. Choose current by acceptance blocking, evidence proximity, information gain and impact, then execution and return cost. Keep current depth-first until confirmed, discriminatively ruled out, explicitly blocked, or strictly outranked by new evidence. Before switching, close it or update its result and resume condition; when possible close the old item and set the next current in the same delta. Every close requires outcome and reason; refs is a separate sibling field. Base close data only on observations already available before the current action runs. For verification TODOs, the reason must identify experiments and observations that distinguish the hypothesis; a single ordinary request, payload miss, scanner miss, or absence of an obvious error is insufficient to close."),
	},
		aitool.WithRawParam("current", map[string]any{"type": []string{"string", "null"}, "description": "Optional unique current TODO id. Omit to keep focus; null or empty clears it."}),
		aitool.WithStructArrayParam("add", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Description("Optional stable id; the engine generates todo-N when omitted. Do not add an existing id again; use update when its text changes.")),
			aitool.WithStringParam("text", aitool.WithParam_Required(true), aitool.WithParam_Description("A short, actionable TODO. A discovered branch should preserve its concrete target, triggering evidence, falsifiable hypothesis, and first resume action.")),
		),
		aitool.WithStructArrayParam("update", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("text", aitool.WithParam_Required(true)),
		),
		aitool.WithStructArrayParam("close", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("outcome", aitool.WithParam_Required(true), aitool.WithParam_EnumString("resolved", "dismissed", "deferred")),
			aitool.WithStringParam("reason", aitool.WithParam_Required(true), aitool.WithParam_Description("Required audit trail string based on observations already available in the current task before this action runs: verified result for resolved; attempts and stop reason for dismissed; attempts, unfinished work, and continuation condition for deferred. Keep refs outside this string as a sibling JSON field. Historical memory or another task's conclusion must be revalidated before resolved.")),
			aitool.WithSimpleArrayParam("refs", "string", aitool.WithParam_Description("Optional array of tool-call or observation references. This is a sibling of reason, not part of the reason key or string.")),
		),
	)
}
