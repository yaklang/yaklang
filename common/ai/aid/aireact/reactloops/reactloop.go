package reactloops

import (
	"bytes"

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

type ContextProviderFunc func(loop *ReActLoop, nonce string) (string, error)
type FeedbackProviderFunc func(loop *ReActLoop, feedback *bytes.Buffer, nonce string) (string, error)

type ReActLoop struct {
	invoker aicommon.AIInvokeRuntime
	config  aicommon.AICallerConfigIf
	emitter *aicommon.Emitter

	maxIterations int

	loopName string

	persistentInstructionProvider   ContextProviderFunc
	reflectionOutputExampleProvider ContextProviderFunc
	reactiveDataBuilder             FeedbackProviderFunc

	allowRAG            func() bool
	allowToolCall       func() bool
	toolsGetter         func() []*aitool.Tool
	allowUserInteract   func() bool
	loopPromptGenerator ReActLoopCoreGenerateCode

	// store variable
	vars *omap.OrderedMap[string, any]

	// ai loop once
	actions      *omap.OrderedMap[string, *LoopAction]
	streamFields *omap.OrderedMap[string, *LoopStreamField]
	aiTagFields  *omap.OrderedMap[string, *LoopAITagField]
}

func (r *ReActLoop) GetInvoker() aicommon.AIInvokeRuntime {
	return r.invoker
}

func (r *ReActLoop) GetEmitter() *aicommon.Emitter {
	return r.emitter
}

func (r *ReActLoop) GetConfig() aicommon.AICallerConfigIf {
	return r.config
}

func NewReActLoop(name string, invoker aicommon.AIInvokeRuntime, options ...ReActLoopOption) (*ReActLoop, error) {
	if utils.IsNil(invoker) {
		return nil, utils.Error("invoker is nil in ReActLoop")
	}

	config := invoker.GetConfig()

	r := &ReActLoop{
		invoker:       invoker,
		loopName:      name,
		config:        config,
		emitter:       config.GetEmitter(),
		maxIterations: 100,
		actions:       omap.NewEmptyOrderedMap[string, *LoopAction](),
		streamFields:  omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:   omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		vars:          omap.NewEmptyOrderedMap[string, any](),
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

	return r, nil
}

func (r *ReActLoop) Set(i string, result any) {
	r.vars.Set(i, result)
}

func (r *ReActLoop) Get(i string) string {
	result, ok := r.vars.Get(i)
	if ok {
		return utils.InterfaceToString(result)
	}
	return ""
}
