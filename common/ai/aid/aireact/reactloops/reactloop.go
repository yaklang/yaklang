package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type ReActLoopCoreGenerateCode func(
	userInput string,
	contextResult string,
	contextFeedback string,
) (string, error)

type ReActLoopOption func(r *ReActLoop)

type ReActLoop struct {
	config  aicommon.AICallerConfigIf
	caller  aicommon.AICaller
	emitter *aicommon.Emitter

	maxIterations int

	loopName string

	allowRAG            func() bool
	allowToolCall       func() bool
	toolsGetter         func() []*aitool.Tool
	allowUserInteract   func() bool
	loopPromptGenerator ReActLoopCoreGenerateCode

	// ai loop once
	actions      *omap.OrderedMap[string, *LoopAction]
	streamFields *omap.OrderedMap[string, *LoopStreamField]
}

func NewReActLoop(name string, config aicommon.AICallerConfigIf, options ...ReActLoopOption) (*ReActLoop, error) {
	if utils.IsNil(config) {
		return nil, utils.Error("config is nil in ReActLoop")
	}

	caller, ok := config.(aicommon.AICaller)
	if ok {
		caller = caller
	} else {
		caller = nil
	}

	r := &ReActLoop{
		loopName:      name,
		config:        config,
		caller:        caller,
		emitter:       config.GetEmitter(),
		maxIterations: 100,
		actions:       omap.NewEmptyOrderedMap[string, *LoopAction](),
		streamFields:  omap.NewEmptyOrderedMap[string, *LoopStreamField](),
	}
	for _, action := range []*LoopAction{
		loopAction_RequireTool,
		loopAction_AskForClarification,
		loopAction_Finish,
	} {
		r.actions.Set(action.ActionType, action)
	}

	for _, streamField := range []*LoopStreamField{
		{
			FieldName: "human_readable_thought",
		},
	} {
		r.streamFields.Set(streamField.FieldName, streamField)
	}

	for _, opt := range options {
		opt(r)
	}

	if r.emitter == nil {
		return nil, utils.Error("loop's emitter is nil in ReActLoop")
	}

	if utils.IsNil(r.caller) {
		return nil, utils.Error("loop's ai caller is nil in ReActLoop")
	}

	return r, nil
}

func (r *ReActLoop) generateSchemaString() (string, error) {
	// loop
	// build in code
	schema := buildSchema(r.actions.Values()...)
	return schema, nil
}
