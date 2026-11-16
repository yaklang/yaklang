package ypb

import "fmt"

const (
	AgentModeSecurity = "security"
	AgentModeCoding   = "coding"
)

// EffectiveAgentMode reads the legacy flag only when AgentMode is absent.
func (p *AIStartParams) EffectiveAgentMode() string {
	if p.GetAgentMode() != "" {
		return p.GetAgentMode()
	}
	if p.GetCodingMode() {
		return AgentModeCoding
	}
	return AgentModeSecurity
}

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

// NormalizeAgentMode keeps the deprecated wire field consistent for old clients.
func (p *AIStartParams) NormalizeAgentMode() error {
	mode, err := NormalizeAgentMode(p.EffectiveAgentMode())
	if err != nil {
		return err
	}
	p.AgentMode = mode
	p.CodingMode = mode == AgentModeCoding
	return nil
}
