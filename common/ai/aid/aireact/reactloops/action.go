package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type LoopActionVerifierFunc func(loop *ReActLoop, action *aicommon.Action) error
type LoopActionHandlerFunc func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator)

type LoopAction struct {
	// plan 与 forge executor 会允许支持异步执行，异步情况下仍然允许对话和其他功能
	AsyncMode      bool
	ActionType     string `json:"type"`
	Description    string `json:"description"`
	Options        []aitool.ToolOption
	ActionVerifier LoopActionVerifierFunc
	ActionHandler  LoopActionHandlerFunc
	StreamFields   []*LoopStreamField
}

func buildSchema(actions ...*LoopAction) string {
	var actionNames []string
	for _, action := range actions {
		actionNames = append(actionNames, action.ActionType)
	}
	var opts []any = []any{
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Description("required '@action' field to identify the action type"),
			aitool.WithParam_EnumString(actionNames...),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"human_readable_thought",
			aitool.WithParam_Description("Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. "),
		),
	}

	existed := make(map[string]struct{})
	existed["@action"] = struct{}{}
	existed["human_readable_thought"] = struct{}{}

	for _, action := range actions {
		if action == nil {
			continue
		}
		if len(action.Options) <= 0 {
			continue
		}
		for _, opt := range action.Options {
			var rawOpt = opt
			opts = append(opts, rawOpt)
		}
	}

	return aitool.NewObjectSchema(opts...)
}
