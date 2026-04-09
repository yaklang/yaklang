package node

import (
	"context"
	"testing"
	"time"
)

type stubRuntimeStatusProvider struct {
	snapshot RuntimeStatus
}

func (s stubRuntimeStatusProvider) Snapshot() RuntimeStatus {
	return s.snapshot
}

type stubSessionTransport struct {
	heartbeatSession SessionState
	heartbeatRequest HeartbeatRequest
	heartbeatCalls   int

	shutdownSession SessionState
	shutdownRequest ShutdownRequest
	shutdownCalls   int
}

func (s *stubSessionTransport) Bootstrap(
	context.Context,
	BootstrapRequest,
) (SessionState, error) {
	return SessionState{}, nil
}

func (s *stubSessionTransport) Heartbeat(
	_ context.Context,
	session SessionState,
	request HeartbeatRequest,
) error {
	s.heartbeatCalls++
	s.heartbeatSession = session
	s.heartbeatRequest = request
	return nil
}

func (s *stubSessionTransport) Shutdown(
	_ context.Context,
	session SessionState,
	request ShutdownRequest,
) error {
	s.shutdownCalls++
	s.shutdownSession = session
	s.shutdownRequest = request
	return nil
}

func TestNodeBaseHeartbeatBuildsRequestFromRuntimeStatus(t *testing.T) {
	t.Parallel()

	activeAttempts := []ActiveAttemptHeartbeat{
		{
			AttemptID:      "attempt-1",
			JobID:          "job-1",
			SubtaskID:      "subtask-1",
			Status:         "running",
			CompletedUnits: 3,
			TotalUnits:     8,
			LastActivityAt: time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC),
		},
	}
	transport := &stubSessionTransport{}
	node := &NodeBase{
		rootCtx:           context.Background(),
		NodeId:            "node-1",
		version:           "dev",
		labels:            map[string]string{"zone": "cn"},
		capabilityKeys:    []string{"yak.execute"},
		maxRunningJobs:    8,
		lifecycleState:    DefaultLifecycleState,
		requestTimeout:    time.Second,
		transport:         transport,
		statusProvider:    stubRuntimeStatusProvider{snapshot: RuntimeStatus{RunningJobs: 2, ActiveAttempts: activeAttempts}},
		heartbeatInterval: 1500 * time.Millisecond,
		session: SessionState{
			SessionID:    "session-1",
			SessionToken: "token-1",
		},
	}

	before := time.Now().UTC()
	if err := node.heartbeat(); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	after := time.Now().UTC()

	if transport.heartbeatCalls != 1 {
		t.Fatalf("unexpected heartbeat call count: %d", transport.heartbeatCalls)
	}
	if transport.heartbeatSession.SessionID != "session-1" {
		t.Fatalf("unexpected session_id: %s", transport.heartbeatSession.SessionID)
	}
	if transport.heartbeatRequest.LifecycleState != DefaultLifecycleState {
		t.Fatalf("unexpected lifecycle_state: %s", transport.heartbeatRequest.LifecycleState)
	}
	if transport.heartbeatRequest.Version != "dev" {
		t.Fatalf("unexpected version: %s", transport.heartbeatRequest.Version)
	}
	if transport.heartbeatRequest.RunningJobs != 2 {
		t.Fatalf("unexpected running_jobs: %d", transport.heartbeatRequest.RunningJobs)
	}
	if transport.heartbeatRequest.MaxRunningJobs != 8 {
		t.Fatalf("unexpected max_running_jobs: %d", transport.heartbeatRequest.MaxRunningJobs)
	}
	if transport.heartbeatRequest.HeartbeatIntervalSeconds != 2 {
		t.Fatalf(
			"unexpected heartbeat_interval_seconds: %d",
			transport.heartbeatRequest.HeartbeatIntervalSeconds,
		)
	}
	if transport.heartbeatRequest.ObservedAt.Before(before) ||
		transport.heartbeatRequest.ObservedAt.After(after) {
		t.Fatalf("unexpected observed_at: %s", transport.heartbeatRequest.ObservedAt)
	}
	if len(transport.heartbeatRequest.ActiveAttempts) != 1 {
		t.Fatalf("unexpected active_attempt count: %d", len(transport.heartbeatRequest.ActiveAttempts))
	}
	if transport.heartbeatRequest.ActiveAttempts[0].AttemptID != "attempt-1" {
		t.Fatalf(
			"unexpected attempt_id: %s",
			transport.heartbeatRequest.ActiveAttempts[0].AttemptID,
		)
	}

	node.labels["zone"] = "us"
	node.capabilityKeys[0] = "mutated"
	activeAttempts[0].AttemptID = "changed"

	if transport.heartbeatRequest.Labels["zone"] != "cn" {
		t.Fatalf("heartbeat labels were not cloned: %v", transport.heartbeatRequest.Labels)
	}
	if transport.heartbeatRequest.CapabilityKeys[0] != "yak.execute" {
		t.Fatalf(
			"heartbeat capability_keys were not cloned: %v",
			transport.heartbeatRequest.CapabilityKeys,
		)
	}
	if transport.heartbeatRequest.ActiveAttempts[0].AttemptID != "attempt-1" {
		t.Fatalf(
			"heartbeat active_attempts were not cloned: %s",
			transport.heartbeatRequest.ActiveAttempts[0].AttemptID,
		)
	}
}

func TestNodeBaseHeartbeatWithoutSessionReturnsError(t *testing.T) {
	t.Parallel()

	node := &NodeBase{
		rootCtx:        context.Background(),
		requestTimeout: time.Second,
		transport:      &stubSessionTransport{},
	}

	if err := node.heartbeat(); err == nil {
		t.Fatal("expected heartbeat error")
	}
}
