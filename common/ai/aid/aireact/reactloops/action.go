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
		aitool.WithParam_Description("The only TODO write channel. Before the next action, add each newly observed, relevant independent target with its own ID; prose, answers and evidence do not register work. Do not put multiple independently verifiable routes/files/records into one 'audit all' TODO, or merge later discoveries into an old item. Supporting files and control experiments for the SAME acceptance target may share its TODO. Start an unknown target set with a discovery TODO; once enumerated, add separate execution TODOs before pursuing them. Apply order: add, update, close, current. Updates preserve the original acceptance goal. Closed IDs are immutable; use a new ID for continuation. Keep current while distinct experiments gain information; at a temporary dead end update it, keep it OPEN, and switch current. Close only on prior observed acceptance evidence, discriminating exclusion, or a real external blocker. An empty list, a primary deliverable, a single failed call or low expected payoff does not prove completion."),
	},
		aitool.WithRawParam("current", map[string]any{"type": []string{"string", "null"}, "description": "Optional unique open TODO id. Closed history cannot be selected; create a new continuation ID instead. Omit to keep focus; null or empty clears it."}),
		aitool.WithStructArrayParam("add", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Description("Optional stable id; the engine generates todo-N when omitted. Do not add an existing ID again. Use update only for an open item; use a new ID for continuation of closed history.")),
			aitool.WithStringParam("text", aitool.WithParam_Required(true), aitool.WithParam_Description("ONE independently verifiable target, its source (user request or Observation), and acceptance check. For an unexplored target use 待探索 with a source, then refine after actual exploration. Never bundle independent objects into 'inspect all', and never equate one read or one negative result with completion.")),
		),
		aitool.WithStructArrayParam("update", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true), aitool.WithParam_Description("ID of an open TODO only; closed history is immutable.")),
			aitool.WithStringParam("text", aitool.WithParam_Required(true), aitool.WithParam_Description("Preserve the original target and acceptance requirement; record actual progress. Add newly discovered independent objects with new IDs instead of merging them here. Editing away unfinished words is not evidence.")),
		),
		aitool.WithStructArrayParam("close", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true), aitool.WithParam_Description("ID of an open TODO only; never re-close terminal history to satisfy finish.")),
			aitool.WithStringParam("outcome", aitool.WithParam_Required(true), aitool.WithParam_EnumString("resolved", "dismissed", "deferred")),
			aitool.WithStringParam("reason", aitool.WithParam_Required(true), aitool.WithParam_Description("Explain actual actions, prior observations, and why they satisfy the original acceptance check. resolved: verified result, never an unexplored target; dismissed: discriminating exclusion, not a single miss; deferred: real external blocker, attempted alternatives, unfinished work and testable recovery condition. Do not close a promise of remaining work. Never pre-credit the tool running in this action. Keep refs separate; cite observations already available.")),
			aitool.WithSimpleArrayParam("refs", "string", aitool.WithParam_Description("Optional array of tool-call or observation references. This is a sibling of reason, not part of the reason key or string.")),
		),
	)
}
