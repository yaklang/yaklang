package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
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

func WithUserInteract(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithUserInteractGetter(func() bool {
			return b[0]
		})
	}
	return WithUserInteractGetter(func() bool {
		return true
	})
}

func WithRegisterLoopAction(actionName string, desc string, opts ...aitool.ToolOption) ReActLoopOption {
	return func(r *ReActLoop) {
		if r.actions.Have(actionName) {
			log.Errorf("loop action %s already registered", actionName)
			return
		}
		r.actions.Set(actionName, &LoopAction{
			ActionType:  actionName,
			Description: desc,
			Options:     opts,
		})
	}

}

func WithMaxIterations(maxIterations int) ReActLoopOption {
	return func(r *ReActLoop) {
		r.maxIterations = maxIterations
	}
}

func WithAICaller(caller aicommon.AICaller) ReActLoopOption {
	return func(r *ReActLoop) {
		r.caller = caller
	}
}
