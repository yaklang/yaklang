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
	// Build oneOf schemas - one schema per action
	var oneOfSchemas [][]aitool.ToolOption

	for _, action := range actions {
		if action == nil {
			continue
		}

		// Build description with metadata if available
		desc := action.Description
		if meta, ok := GetLoopMetadata(action.ActionType); ok && meta.UsagePrompt != "" {
			desc = meta.UsagePrompt
		}

		// Build the schema for this specific action
		actionSchemaOpts := []aitool.ToolOption{
			// Fixed @action field with const value for this specific action
			aitool.WithStringParam(
				"@action",
				aitool.WithParam_Description("Action type: "+desc),
				aitool.WithParam_EnumString(action.ActionType),
				aitool.WithParam_Required(true),
			),
			// identifier field
			aitool.WithStringParam(
				"identifier",
				aitool.WithParam_Description(
					"REQUIRED. A short snake_case label (lowercase + underscores, <=30 chars) describing the PURPOSE of this action call. "+
						"Examples: folder_skeleton, read_go_mod, grep_sql_exec, write_dir_structure. "+
						"This identifier is used in log file paths to help users quickly understand what each action call is doing.",
				),
				aitool.WithParam_Required(true),
			),
			// human_readable_thought field
			aitool.WithStringParam(
				"human_readable_thought",
				aitool.WithParam_Description(
					"Optional. Omit this field when @action is 'directly_answer' or when the next step is already obvious. If you do provide it, keep it to one short, action-oriented sentence only (prefer <=12 Chinese characters or <=8 English words).",
				),
			),
		}

		// Add action-specific options
		if len(action.Options) > 0 {
			actionSchemaOpts = append(actionSchemaOpts, action.Options...)
		}

		oneOfSchemas = append(oneOfSchemas, actionSchemaOpts)
	}

	// Build the root schema with oneOf at the top level
	return aitool.NewOneOfObjectSchema(oneOfSchemas...)
}
