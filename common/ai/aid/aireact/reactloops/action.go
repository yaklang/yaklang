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
		aitool.WithParam_Description("The only write channel for the short-term TODO work set; TODO LIST, prose, and custom TODO tags are read-only and ignored for state changes. This field is optional only when state truly does not change. Add, refine, close, schedule a continuation, or switch TODOs in the same action JSON that advances the work. Apply order: add, update, close, current. Update, close, and current apply only to open items. Closed IDs are immutable audit history: to continue a deferred or weakly closed item, add a new TODO with a new ID and make that continuation current. Open items form the Frontier and one item is current. Before following one branch, record every concrete in-scope branch exposed by an Observation. A discovered link, form action, redirect, script route, documented endpoint, or response field is sufficient source evidence for a coverage TODO; require a falsifiable hypothesis only for a verification claim. Keep current while materially different experiments can gain information. A tool/parameter/transport/auth failure or one payload miss is not closure: correct it or vary the controllable channel first. When current completes, is discriminatively ruled out, is externally blocked, or temporarily has zero information gain, save its result or continuation condition and set the next Frontier item in the same delta. Never close or defer items merely to pass finish. Every close requires outcome and reason; refs is a sibling field and closure may use only observations already available before this action."),
	},
		aitool.WithRawParam("current", map[string]any{"type": []string{"string", "null"}, "description": "Optional unique open TODO id. Closed history cannot be selected; create a new continuation ID instead. Omit to keep focus; null or empty clears it."}),
		aitool.WithStructArrayParam("add", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Description("Optional stable id; the engine generates todo-N when omitted. Do not add an existing ID again. Use update only for an open item; use a new ID for continuation of closed history.")),
			aitool.WithStringParam("text", aitool.WithParam_Required(true), aitool.WithParam_Description("A short, actionable TODO. Preserve the concrete target, source evidence, and first resume action; add a falsifiable hypothesis when the item verifies a claim.")),
		),
		aitool.WithStructArrayParam("update", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true), aitool.WithParam_Description("ID of an open TODO only; closed history is immutable.")),
			aitool.WithStringParam("text", aitool.WithParam_Required(true)),
		),
		aitool.WithStructArrayParam("close", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true), aitool.WithParam_Description("ID of an open TODO only; never re-close terminal history to satisfy finish.")),
			aitool.WithStringParam("outcome", aitool.WithParam_Required(true), aitool.WithParam_EnumString("resolved", "dismissed", "deferred")),
			aitool.WithStringParam("reason", aitool.WithParam_Required(true), aitool.WithParam_Description("Required audit trail string based on observations already available in the current task before this action runs: verified result for resolved; attempts and stop reason for dismissed; attempts, unfinished work, and continuation condition for deferred. Keep refs outside this string as a sibling JSON field. Historical memory or another task's conclusion must be revalidated before resolved.")),
			aitool.WithSimpleArrayParam("refs", "string", aitool.WithParam_Description("Optional array of tool-call or observation references. This is a sibling of reason, not part of the reason key or string.")),
		),
	)
}
