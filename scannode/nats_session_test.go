package scannode

import (
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/node"
)

func TestSessionScopedOutboundSubject(t *testing.T) {
	session := node.SessionState{CompanyID: "company-1", SessionID: "session-1"}
	tests := map[string]string{
		"legion.event.job.started":                    "legion.event.node.session-1.job.started",
		"legion.hids.observation":                     "legion.hids.node.session-1.observation",
		"legion.realtime.ssa.debug.result.v2.query-1": "legion.realtime.node.session-1.ssa.debug.result.v2.query-1",
		"legion.event.node.session-1.job.started":     "legion.event.node.session-1.job.started",
	}
	for input, want := range tests {
		got, err := sessionScopedOutboundSubject(session, input)
		if err != nil {
			t.Fatalf("scope %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("scope %q = %q, want %q", input, got, want)
		}
	}
	for _, input := range []string{
		"legion.event.node.session-2.job.started",
		"legion.command.node.node-1.job.dispatch",
	} {
		if _, err := sessionScopedOutboundSubject(session, input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestNATSSessionConnectionKeyChangesOnCredentialRotation(t *testing.T) {
	base := node.SessionState{
		CompanyID: "company-1", SessionID: "session-1", NATSURL: "tls://nats.example:4222",
		NATSCredentials: "credentials-1", NATSCredentialsExpiresAt: time.Now().Add(time.Hour),
		InboxPrefix: "_INBOX.node.session-1", ExpiresAt: time.Now().Add(time.Hour),
	}
	rotated := base
	rotated.NATSCredentials = "credentials-2"
	if natsSessionConnectionKey(base) == natsSessionConnectionKey(rotated) {
		t.Fatal("credential rotation must change the NATS connection key")
	}
}

func TestNodeRefForSessionIncludesCompany(t *testing.T) {
	ref := nodeRefForSession("node-1", node.SessionState{SessionID: "session-1", CompanyID: "company-1"})
	if ref.GetNodeId() != "node-1" || ref.GetNodeSessionId() != "session-1" || ref.GetCompanyId() != "company-1" {
		t.Fatalf("node ref = %+v", ref)
	}
}

func TestConnectNATSForCompanySessionRejectsInvalidCredentialsBeforeDial(t *testing.T) {
	session := node.SessionState{
		CompanyID: "company-1", SessionID: "session-1", NATSURL: "tls://127.0.0.1:1",
		NATSCredentials: "not-a-creds-file", NATSCredentialsExpiresAt: time.Now().Add(time.Hour),
		InboxPrefix: "_INBOX.node.session-1", ExpiresAt: time.Now().Add(time.Hour),
	}
	_, err := connectNATSForSession(session, "test")
	if err == nil || !strings.Contains(err.Error(), "parse company nats user") {
		t.Fatalf("invalid credentials error = %v", err)
	}
}
