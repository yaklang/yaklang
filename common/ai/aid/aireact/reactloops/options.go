package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func WithLoopPromptGenerator(generator ReActLoopCoreGenerateCode) ReActLoopOption {
	return func(r *ReActLoop) {
		r.loopPromptGenerator = generator
	}
}

func WithAllowRAGGetter(allowRAG func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowRAG = allowRAG
	}
}

func WithAllowAIForgeGetter(allowAIForge func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowAIForge = allowAIForge
	}
}

func WithAllowPlanAndExecGetter(allowPlanAndExec func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowPlanAndExec = allowPlanAndExec
	}
}

func WithAllowPlanAndExec(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowPlanAndExecGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowPlanAndExecGetter(func() bool {
		return true
	})
}

func WithAllowAIForge(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowAIForgeGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowAIForgeGetter(func() bool {
		return true
	})
}

func WithAllowRAG(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowRAGGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowRAGGetter(func() bool {
		return true
	})
}

func WithAllowToolCallGetter(allowToolCall func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowToolCall = allowToolCall
	}
}

func WithAllowToolCall(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowToolCallGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowToolCallGetter(func() bool {
		return true
	})
}

func WithToolsGetter(getter func() []*aitool.Tool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.toolsGetter = getter
	}
}

func WithUserInteractGetter(allowUserInteract func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowUserInteract = allowUserInteract
	}
}

func WithAllowUserInteract(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithUserInteractGetter(func() bool {
			return b[0]
		})
	}
	return WithUserInteractGetter(func() bool {
		return true
	})
}

func WithRegisterLoopAction(actionName string, desc string, opts []aitool.ToolOption, verifier LoopActionVerifierFunc, handler LoopActionHandlerFunc) ReActLoopOption {
	return WithRegisterLoopActionWithStreamField(actionName, desc, opts, nil, verifier, handler)
}

func WithRegisterLoopActionWithStreamField(actionName string, desc string, opts []aitool.ToolOption, fields []*LoopStreamField, verifier LoopActionVerifierFunc, handler LoopActionHandlerFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		if r.actions.Have(actionName) {
			log.Errorf("loop action %s already registered", actionName)
			return
		}
		r.actions.Set(actionName, &LoopAction{
			AsyncMode:      false,
			ActionType:     actionName,
			Description:    desc,
			Options:        opts,
			ActionVerifier: verifier,
			ActionHandler:  handler,
			StreamFields:   fields,
		})
	}
}

func WithMaxIterations(maxIterations int) ReActLoopOption {
	return func(r *ReActLoop) {
		r.maxIterations = maxIterations
	}
}

func WithAITagField(tagName, variableName string) ReActLoopOption {
	return func(r *ReActLoop) {
		if r.aiTagFields == nil {
			r.aiTagFields = omap.NewEmptyOrderedMap[string, *LoopAITagField]()
		}
		r.aiTagFields.Set(tagName, &LoopAITagField{
			TagName:      tagName,
			VariableName: variableName,
		})
	}
}

func WithAITagFieldWithAINodeId(tagName, variableName, nodeId string) ReActLoopOption {
	return func(r *ReActLoop) {
		if r.aiTagFields == nil {
			r.aiTagFields = omap.NewEmptyOrderedMap[string, *LoopAITagField]()
		}
		r.aiTagFields.Set(tagName, &LoopAITagField{
			TagName:      tagName,
			VariableName: variableName,
			AINodeId:     nodeId,
		})
	}
}

func WithReflectionOutputExampleContextProvider(provider ContextProviderFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		r.reflectionOutputExampleProvider = provider
	}
}

func WithPersistentContextProvider(provider ContextProviderFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		r.persistentInstructionProvider = provider
	}
}

func WithReflectionOutputExample(example string) ReActLoopOption {
	return WithReflectionOutputExampleContextProvider(func(loop *ReActLoop, nonce string) (string, error) {
		_, result, err := loop.getRenderInfo()
		if err != nil {
			return "", utils.Errorf("get basic prompt info failed: %v", err)
		}
		result["Nonce"] = nonce
		return utils.RenderTemplate(example, result)
	})
}

func WithPersistentInstruction(instruction string) ReActLoopOption {
	return WithPersistentContextProvider(func(loop *ReActLoop, nonce string) (string, error) {
		_, result, err := loop.getRenderInfo()
		if err != nil {
			return "", utils.Errorf("get basic prompt info failed: %v", err)
		}
		result["Nonce"] = nonce
		return utils.RenderTemplate(instruction, result)
	})
}

func WithReactiveDataBuilder(provider FeedbackProviderFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		r.reactiveDataBuilder = provider
	}
}

func WithOnTaskCreated(fn func(task aicommon.AIStatefulTask)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.onTaskCreated = fn
	}
}

func WithOnAsyncTaskTrigger(fn func(i *LoopAction, task aicommon.AIStatefulTask)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.onAsyncTaskTrigger = fn
	}
}

func WithActionFactoryFromLoop(name string) ReActLoopOption {
	return func(r *ReActLoop) {
		factory, ok := GetLoopFactory(name)
		if !ok {
			log.Errorf("reactloop[%v] not found", name)
			return
		}
		actionFac := ConvertReActLoopFactoryToActionFactory(name, factory)
		r.loopActions.Set(name, actionFac)
	}
}
