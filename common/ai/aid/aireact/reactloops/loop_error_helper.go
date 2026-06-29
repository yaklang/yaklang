package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const LoopVarLastAIDecisionResponse = "last_ai_decision_response"

// ErrorWithLastAIResponse appends the loop's last AI decision response to the error
// message so upstream handlers (e.g. EmitReActFail) can surface what the model returned.
func ErrorWithLastAIResponse(loop *ReActLoop, msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "react loop failed"
	}
	if loop == nil {
		return utils.Error(msg)
	}
	aiOutput := strings.TrimSpace(loop.Get(LoopVarLastAIDecisionResponse))
	if aiOutput == "" {
		return utils.Error(msg)
	}
	return utils.Errorf("%s; ai_output: %s", msg, aiOutput)
}
