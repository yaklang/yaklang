package node

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func validCompanySession(now time.Time) SessionState {
	return SessionState{
		NodeID:                   "node-1",
		SessionID:                "session-1",
		SessionStartedAt:         now.Add(-time.Minute),
		SessionToken:             "token-1",
		CompanyID:                "company-1",
		NATSURL:                  "tls://nats.example:4222",
		NATSCredentials:          "decorated-creds",
		NATSCredentialsExpiresAt: now.Add(time.Hour),
		CommandStream:            "LEGION_COMMANDS",
		CommandConsumer:          "node_session_1",
		InboxPrefix:              "_INBOX.node.session-1",
		CommandSubject:           "legion.command.node.node-1",
		EventSubjectPrefix:       "legion.event.node.session-1",
		ExpiresAt:                now.Add(2 * time.Hour),
	}
}

func TestValidateSessionStateRequiresCompleteCompanyTransport(t *testing.T) {
	now := time.Now().UTC()
	session := validCompanySession(now)
	if err := validateSessionState(session, now); err != nil {
		t.Fatalf("valid company session rejected: %v", err)
	}

	session.SessionStartedAt = time.Time{}
	if err := validateSessionState(session, now); err == nil || !strings.Contains(err.Error(), "session_started_at") {
		t.Fatalf("missing server session boundary error = %v", err)
	}
	session = validCompanySession(now)
	session.CommandConsumer = ""
	if err := validateSessionState(session, now); err == nil || !strings.Contains(err.Error(), "command_consumer") {
		t.Fatalf("missing server consumer error = %v", err)
	}

	session = validCompanySession(now)
	session.NATSCredentialsExpiresAt = now
	if err := validateSessionState(session, now); err == nil || !strings.Contains(err.Error(), "nats_credentials_expires_at") {
		t.Fatalf("expired credentials error = %v", err)
	}
}

func TestApplyHeartbeatSessionRotatesCredentialsAtomically(t *testing.T) {
	now := time.Now().UTC()
	node := &NodeBase{session: validCompanySession(now)}
	err := node.applyHeartbeatSession(HeartbeatResponse{
		CompanyID:                "company-1",
		NATSCredentials:          "rotated-creds",
		NATSCredentialsExpiresAt: now.Add(90 * time.Minute),
		CommandConsumer:          "node_session_rotated",
		ExpiresAt:                now.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("apply heartbeat renewal: %v", err)
	}
	session, ok := node.GetSessionState()
	if !ok {
		t.Fatal("renewed session unavailable")
	}
	if session.NATSCredentials != "rotated-creds" || session.CommandConsumer != "node_session_rotated" {
		t.Fatalf("renewed session = %+v", session)
	}
	if session.CommandStream != "LEGION_COMMANDS" || session.InboxPrefix != "_INBOX.node.session-1" {
		t.Fatalf("partial heartbeat erased session fields: %+v", session)
	}
}

func TestApplyHeartbeatSessionRejectsCompanySwitch(t *testing.T) {
	now := time.Now().UTC()
	node := &NodeBase{session: validCompanySession(now)}
	if err := node.applyHeartbeatSession(HeartbeatResponse{CompanyID: "company-2"}); err == nil {
		t.Fatal("expected company switch to be rejected")
	}
	session, _ := node.GetSessionState()
	if session.CompanyID != "company-1" {
		t.Fatalf("company changed after rejected heartbeat: %q", session.CompanyID)
	}
}

func TestResilienceCompanySessionInvalidationHookRunsAfterClearingIdentity(t *testing.T) {
	base := &NodeBase{session: validCompanySession(time.Now()), boundCompanyID: "company-1"}
	calls := 0
	base.SetSessionInvalidatedHook(func(previous SessionState) {
		calls++
		if previous.CompanyID != "company-1" {
			t.Fatal("lost previous company binding")
		}
		if _, ok := base.GetSessionState(); ok {
			t.Fatal("runtime cleanup observed a live revoked identity")
		}
	})
	base.clearSession()
	base.clearSession()
	if calls != 1 || base.boundCompanyID != "company-1" {
		t.Fatal("invalidation lost binding or ran twice")
	}
}

func TestResilienceCompanyBootstrapCannotDowngradeAfterRevocation(t *testing.T) {
	transport := &stubBootstrapTransport{session: SessionState{NodeID: "node-1", SessionID: "legacy", SessionToken: "token"}}
	base := &NodeBase{rootCtx: context.Background(), requestTimeout: time.Second, transport: transport, boundCompanyID: "company-1"}
	if err := base.bootstrapSession(); err == nil {
		t.Fatal("revoked company node downgraded to an unscoped transport")
	}
	if _, ok := base.GetSessionState(); ok {
		t.Fatal("rejected bootstrap established identity")
	}
	transport.session = validCompanySession(time.Now())
	transport.session.CompanyID = "company-2"
	if err := base.bootstrapSession(); err == nil {
		t.Fatal("node switched companies")
	}
}

func TestCompanyBootstrapPreservesServerSessionBoundary(t *testing.T) {
	started := time.Date(2026, 10, 8, 8, 0, 0, 123456789, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"node_id":"node-1","node_session_id":"session-1","company_id":"company-1","session_started_at":%q}`, started.Format(time.RFC3339Nano))
	}))
	defer server.Close()
	transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	session, err := transport.Bootstrap(context.Background(), BootstrapRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !session.SessionStartedAt.Equal(started) {
		t.Fatalf("server boundary changed: %v", session.SessionStartedAt)
	}
}
