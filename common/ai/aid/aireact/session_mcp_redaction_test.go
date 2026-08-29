package aireact

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactedSessionMCPMountFailureDropsTransportSecrets(t *testing.T) {
	secret := "signed-shard-token"
	message := redactedSessionMCPMountFailure(
		"ssa_risk_ai_judgement\nforged-log-line",
		errors.New("dial http://host/v1/ai/legacy/"+secret+"/sse: lookup failed"),
	)
	if strings.Contains(message, secret) || strings.Contains(message, "http://") ||
		strings.Contains(message, "\nforged-log-line") {
		t.Fatalf("session MCP failure was not safely redacted: %q", message)
	}
	if !strings.Contains(message, "connection details redacted") {
		t.Fatalf("session MCP failure lost its safe operator summary: %q", message)
	}
}
