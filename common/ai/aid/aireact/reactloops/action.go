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
