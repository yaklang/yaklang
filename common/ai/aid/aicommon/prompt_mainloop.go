package aicommon

import "github.com/yaklang/yaklang/common/yakgrpc/ypb"

// AgentPromptSet owns mode-specific content. Protocol, cache sections and caller
// schemas remain shared. Returned values contain immutable strings only.
type AgentPromptSet struct {
	textHighStatic            string
	functionHighStatic        string
	DefaultInstruction        string
	DefaultOutputExample      string
	VerificationInstruction   string
	VerificationOutputExample string
	WorkspaceTemplate         string
	PlanReviewInstruction     string
	PlanReviewOutputExample   string
	TaskReviewInstruction     string
	TaskReviewOutputExample   string
}

func (p AgentPromptSet) HighStatic(protocol PromptProtocol) string {
	if protocol == PromptProtocolFunctionCall {
		return p.functionHighStatic
	}
	return p.textHighStatic
}

var (
	securityPrompts = loadSecurityPrompts()
	codingPrompts   = loadCodingPrompts()
)

// AgentPrompts is the single content selector. Config/start-parameter validation
// rejects unsupported modes before prompt assembly; an unspecified mode is security.
func AgentPrompts(mode string) AgentPromptSet {
	if mode == ypb.AgentModeCoding {
		return codingPrompts
	}
	return securityPrompts
}
