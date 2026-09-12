package node

import (
	"context"
	"fmt"
	"github.com/tevino/abool"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHeartbeatResourcePolicyResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body      string
		policy, invalid bool
	}{
		{"legacy empty", "", false, false},
		{"legacy object", "{}", false, false},
		{"zero is explicit", `{"resource_policy":{"system_reserved_cpu_millicores":0,"system_reserved_memory_bytes":0,"max_running_jobs":0}}`, true, false},
		{"configured", `{"resource_policy":{"system_reserved_cpu_millicores":500,"system_reserved_memory_bytes":1024,"max_running_jobs":2}}`, true, false},
		{"malformed", `{`, false, true},
		{"incomplete", `{"resource_policy":{"max_running_jobs":2}}`, false, true},
		{"negative", `{"resource_policy":{"system_reserved_cpu_millicores":-1,"system_reserved_memory_bytes":0,"max_running_jobs":0}}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer token" {
					t.Error("missing session authentication")
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.(ResourcePolicyTransport).HeartbeatWithResponse(context.Background(), SessionState{SessionID: "session", SessionToken: "token"}, HeartbeatRequest{})
			if (err != nil) != tc.invalid {
				t.Fatalf("error=%v", err)
			}
			if !tc.invalid && (response.ResourcePolicy != nil) != tc.policy {
				t.Fatalf("response=%+v", response)
			}
		})
	}
}

type policyStatusProvider struct {
	maximum uint32
	reject  bool
}

func (p *policyStatusProvider) Snapshot() RuntimeStatus {
	return RuntimeStatus{MaxRunningJobs: p.maximum}
}
func (p *policyStatusProvider) ApplyResourcePolicy(policy ResourcePolicy) error {
	if p.reject {
		return fmt.Errorf("invalid reserve")
	}
	p.maximum = policy.MaxRunningJobs
	return nil
}

type policyTestTransport struct {
	stubSessionTransport
	response HeartbeatResponse
}

func (s *policyTestTransport) HeartbeatWithResponse(_ context.Context, _ SessionState, request HeartbeatRequest) (HeartbeatResponse, error) {
	s.heartbeatRequest = request
	return s.response, nil
}
func TestNodeHeartbeatRequiresPolicyAndReportsEffectiveUnlimited(t *testing.T) {
	provider := &policyStatusProvider{maximum: 3}
	transport := &policyTestTransport{}
	n := &NodeBase{rootCtx: context.Background(), requestTimeout: time.Second, transport: transport, statusProvider: provider, maxRunningJobs: 3, session: SessionState{SessionID: "session", SessionToken: "token", ResourcePolicyRequired: true}}
	if err := n.heartbeat(); err == nil {
		t.Fatal("required policy absent")
	}
	transport.response.ResourcePolicy = &ResourcePolicy{}
	if err := n.heartbeat(); err != nil {
		t.Fatal(err)
	}
	if err := n.heartbeat(); err != nil {
		t.Fatal(err)
	}
	if transport.heartbeatRequest.MaxRunningJobs != 0 {
		t.Fatal("effective unlimited fell back to environment maximum")
	}
	provider.reject = true
	transport.response.ResourcePolicy.MaxRunningJobs = 5
	if err := n.heartbeat(); err == nil {
		t.Fatal("invalid policy accepted")
	}
	if provider.maximum != 0 {
		t.Fatal("last valid policy lost")
	}
}

func TestInvalidManagedPolicyRetainsRegisteredSession(t *testing.T) {
	provider := &policyStatusProvider{maximum: 2, reject: true}
	transport := &policyTestTransport{response: HeartbeatResponse{ResourcePolicy: &ResourcePolicy{MaxRunningJobs: 3}}}
	n := &NodeBase{rootCtx: context.Background(), requestTimeout: time.Second, transport: transport, statusProvider: provider, isRegistered: abool.NewBool(true), session: SessionState{SessionID: "session", SessionToken: "token", ResourcePolicyRequired: true}}
	if err := n.heartbeat(); err != nil {
		t.Fatalf("invalid update must not trigger session replacement: %v", err)
	}
	if provider.maximum != 2 || !n.IsRegistered() {
		t.Fatal("last valid policy/session lost")
	}
}

func TestBootstrapResourcePolicyRequirement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"node_id":"node","node_session_id":"session","session_token":"token","resource_policy_required":true}`))
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
	if !session.ResourcePolicyRequired {
		t.Fatal("bootstrap resource policy requirement lost")
	}
}
