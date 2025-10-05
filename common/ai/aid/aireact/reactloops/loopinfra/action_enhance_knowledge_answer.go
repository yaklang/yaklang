package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

var loopAction_EnhanceKnowledgeAnswer = &reactloops.LoopAction{
	ActionType:  `knowledge_enhance_answer`,
	Description: `Enhance the answer with additional knowledge`,
	Options:     []aitool.ToolOption{},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		return
	},
}
