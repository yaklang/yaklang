package ypb

import "fmt"

const (
	AgentModeSecurity = "security"
	AgentModeCoding   = "coding"
)

func NormalizeAgentMode(mode string) (string, error) {
	switch mode {
	case "", AgentModeSecurity:
		return AgentModeSecurity, nil
	case AgentModeCoding:
		return AgentModeCoding, nil
	default:
		return "", fmt.Errorf("unsupported agent mode: %q", mode)
	}
}

// NormalizeAgentMode validates the session mode and applies the security default.
func (p *AIStartParams) NormalizeAgentMode() error {
	mode, err := NormalizeAgentMode(p.GetAgentMode())
	if err != nil {
		return err
	}
	p.AgentMode = mode
	return nil
}
