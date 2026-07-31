package aireact

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// RenderVerificationTodoSnapshot exposes the session work set to verification
// as read-only context. Verification never mutates TODO state.
func (r *ReAct) RenderVerificationTodoSnapshot() string {
	if r == nil || r.config == nil {
		return "- no tracked TODO items"
	}
	rendered := r.config.GetVerificationTodoRendered(aicommon.BuildVerificationTodoScope(r.GetCurrentTask()))
	if rendered == "" {
		return "- no tracked TODO items"
	}
	return rendered
}

func (r *ReAct) RenderVerificationTodoMarkdownSnapshot(_ *aicommon.VerifySatisfactionResult) string {
	return r.RenderVerificationTodoSnapshot()
}
