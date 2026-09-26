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
	AsyncMode   bool
	ActionType  string `json:"type"`
	Description string `json:"description"`
	Options     []aitool.ToolOption
	// NativeDescription and NativeOptions replace text-output instructions when
	// this action is exposed as a provider function. Nil NativeOptions keeps the
	// ordinary Options; the text schema is never changed by these overrides.
	NativeDescription string              `json:"-"`
	NativeOptions     []aitool.ToolOption `json:"-"`
	// NativeOnlyOptions omits common action metadata fields from a dedicated
	// provider tool whose name already identifies its purpose.
	NativeOnlyOptions bool `json:"-"`
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
		if action == nil {
			continue
		}
		actionNames = append(actionNames, action.ActionType)
		actionDesc = append(actionDesc, action.ActionType+": "+actionDescription(action))
	}
	opts := []any{
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Description("required '@action' field to identify the action type"),
			aitool.WithParam_EnumString(actionNames...),
			aitool.WithParam_Required(true),
			aitool.WithParam_Raw("x-@action-rules", actionDesc),
		),
	}
	for _, opt := range commonActionSchemaOptions(false) {
		opts = append(opts, opt)
	}
	for _, action := range actions {
		if action != nil {
			for _, opt := range action.Options {
				opts = append(opts, opt)
			}
		}
	}

	return aitool.NewObjectSchema(opts...)
}

func actionDescription(action *LoopAction) string {
	if meta, ok := GetLoopMetadata(action.ActionType); ok && meta.UsagePrompt != "" {
		return meta.UsagePrompt
	}
	return action.Description
}

// nativeActionDescription keeps provider tool descriptions independent of the
// text-only UsagePrompt. Most actions have a protocol-neutral Description;
// actions whose text description mentions JSON/AITAG override it explicitly.
func nativeActionDescription(action *LoopAction) string {
	if action.NativeDescription != "" {
		return action.NativeDescription
	}
	if action.Description != "" {
		return action.Description
	}
	return "Run the " + action.ActionType + " action using its tool arguments."
}

// withNativeActionDescription gives registered and dynamically constructed
// actions an explicit native description without mutating shared templates.
func withNativeActionDescription(action *LoopAction) *LoopAction {
	if action == nil || action.NativeDescription != "" {
		return action
	}
	copy := *action
	copy.NativeDescription = nativeActionDescription(action)
	return &copy
}

// Native functions keep common identity/display fields, while text actions
// retain the optional TODO sidecar. Native TODO updates have one dedicated tool.
func commonActionSchemaOptions(native bool) []aitool.ToolOption {
	thoughtDescription := "Optional. Omit this field when @action is 'directly_answer' or when the next step is already obvious. If you do provide it, keep it to one short, action-oriented sentence only (prefer <=12 Chinese characters or <=8 English words)."
	if native {
		thoughtDescription = "Optional. Omit for directly_answer or an obvious next step. Otherwise use one short, action-oriented sentence (prefer <=12 Chinese characters or <=8 English words)."
	}
	opts := []aitool.ToolOption{
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
			aitool.WithParam_Description(thoughtDescription),
		),
	}
	if !native {
		opts = append(opts, todoDeltaSchemaOption(false, false))
	}
	return opts
}

func todoDeltaSchemaOption(native, required bool) aitool.ToolOption {
	description := "The only write channel for the short-term TODO work set; TODO LIST, prose, and custom TODO tags are read-only and ignored for state changes. This field is optional only when state truly does not change. Add, refine, close, schedule a continuation, or switch TODOs in the same action JSON that advances the work. Apply order: add, update, close, current. Update, close, and current apply only to open items. Closed IDs are immutable audit history: to continue a deferred or weakly closed item, add a new TODO with a new ID and make that continuation current. Open items form the Frontier and one item is current. Before following one branch, record every concrete in-scope branch exposed by an Observation. A discovered link, form action, redirect, script route, documented endpoint, or response field is sufficient source evidence for a coverage TODO; require a falsifiable hypothesis only for a verification claim. Keep current while materially different experiments can gain information. A tool/parameter/transport/auth failure or one payload miss is not closure: correct it or vary the controllable channel first. When current completes, is discriminatively ruled out, is externally blocked, or temporarily has zero information gain, save its result or continuation condition and set the next Frontier item in the same delta. Never close or defer items merely to pass finish. Every close requires outcome and reason; refs is a sibling field and closure may use only observations already available before this action."
	currentDescription := "Optional unique open TODO id. Closed history cannot be selected; create a new continuation ID instead. Omit to keep focus; null or empty clears it."
	addIDDescription := "Optional stable id; the engine generates todo-N when omitted. Do not add an existing ID again. Use update only for an open item; use a new ID for continuation of closed history."
	addTextDescription := "A short, actionable TODO. Preserve the concrete target, source evidence, and first resume action; add a falsifiable hypothesis when the item verifies a claim."
	updateIDDescription := "ID of an open TODO only; closed history is immutable."
	closeIDDescription := "ID of an open TODO only; never re-close terminal history to satisfy finish."
	closeReasonDescription := "Required audit trail string based on observations already available in the current task before this action runs: verified result for resolved; attempts and stop reason for dismissed; attempts, unfinished work, and continuation condition for deferred. Keep refs outside this string as a sibling JSON field. Historical memory or another task's conclusion must be revalidated before resolved."
	refsDescription := "Optional array of tool-call or observation references. This is a sibling of reason, not part of the reason key or string."
	if native {
		// The shared high-static TODO policy already carries the full rules once.
		// Keep the native schema focused on field shape and local constraints.
		description = "Optional TODO changes in this action's arguments; omit when unchanged. The only TODO write channel."
		currentDescription = "Open TODO id; omit to keep focus, null or empty clears it."
		addIDDescription = "Optional new id; specify it when setting current in this call."
		addTextDescription = "Concrete target, source, and acceptance or resume step."
		updateIDDescription = "Open TODO id."
		closeIDDescription = "Open TODO id."
		closeReasonDescription = "Evidence-based reason for the outcome, using prior observations; put references in refs."
		refsDescription = "Optional observation or tool-call references."
	}
	properties := []aitool.PropertyOption{aitool.WithParam_Description(description)}
	if required {
		properties = append(properties, aitool.WithParam_Required(true))
	}
	return aitool.WithStructParam("todo_delta", properties,
		aitool.WithRawParam("current", map[string]any{"type": []string{"string", "null"}, "description": currentDescription}),
		aitool.WithStructArrayParam("add", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Description(addIDDescription)),
			aitool.WithStringParam("text", aitool.WithParam_Required(true), aitool.WithParam_Description(addTextDescription)),
		),
		aitool.WithStructArrayParam("update", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true), aitool.WithParam_Description(updateIDDescription)),
			aitool.WithStringParam("text", aitool.WithParam_Required(true)),
		),
		aitool.WithStructArrayParam("close", nil, nil,
			aitool.WithStringParam("id", aitool.WithParam_Required(true), aitool.WithParam_Description(closeIDDescription)),
			aitool.WithStringParam("outcome", aitool.WithParam_Required(true), aitool.WithParam_EnumString("resolved", "dismissed", "deferred")),
			aitool.WithStringParam("reason", aitool.WithParam_Required(true), aitool.WithParam_Description(closeReasonDescription)),
			aitool.WithSimpleArrayParam("refs", "string", aitool.WithParam_Description(refsDescription)),
		),
	)
}
