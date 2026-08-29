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
	if !strings.Contains(message, "class=transport_failed") {
		t.Fatalf("session MCP failure lost its bounded diagnostic class: %q", message)
	}
}

func TestClassifySessionMCPMountFailureDoesNotReturnTransportDetails(t *testing.T) {
	tests := []struct {
		err  string
		want string
	}{
		{"create mcp client failed: unexpected status code: 401", "sse_unauthorized"},
		{"initialize mcp client failed: request failed with status 401: signed-secret", "message_unauthorized"},
		{"initialize mcp client failed: request failed with status 404: private-body", "message_not_found"},
		{"dial tcp 127.0.0.1:8080: connect: connection refused", "connect_failed"},
		{"context deadline exceeded for https://private.invalid", "timeout"},
	}
	for _, test := range tests {
		got := classifySessionMCPMountFailure(errors.New(test.err))
		if got != test.want {
			t.Fatalf("classify %q = %q, want %q", test.err, got, test.want)
		}
		if strings.Contains(got, "secret") || strings.Contains(got, "private") || strings.Contains(got, "http") {
			t.Fatalf("classification leaked transport details: %q", got)
		}
	}
}
