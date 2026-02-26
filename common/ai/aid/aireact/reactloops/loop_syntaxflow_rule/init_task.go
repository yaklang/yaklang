package loop_syntaxflow_rule

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// buildInitTask creates the initialization task handler for SyntaxFlow rule loop.
// The SyntaxFlow loop has no requirement analysis phase (unlike Yaklang's grep/RAG search).
// It simply continues to the main ReAct loop for rule generation.
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		log.Infof("[*] React: SyntaxFlow rule loop initialized, waiting for AI to generate rule")
		operator.Continue()
	}
}
