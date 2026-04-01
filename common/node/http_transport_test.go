package node

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTransportBootstrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   int
		response string
		wantErr  bool
	}{
		{
			name:   "success",
			status: http.StatusCreated,
			response: `{
				"node_session_id":"session-1",
				"session_token":"token-1",
				"nats_url":"nats://127.0.0.1:4222",
				"command_subject":"legion.node.cmd.node-1",
				"event_subject_prefix":"legion.node.event",
				"expires_at":"2026-03-27T10:00:00Z"
			}`,
		},
		{
			name:     "server error",
			status:   http.StatusUnauthorized,
			response: `{"error":"invalid enrollment token"}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != bootstrapEndpointPath {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}

				var request BootstrapRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if request.NodeID != "node-1" {
					t.Fatalf("unexpected node_id: %s", request.NodeID)
				}

				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("create transport: %v", err)
			}

			session, err := transport.Bootstrap(context.Background(), BootstrapRequest{
				EnrollmentToken: "enroll-1",
				NodeID:          "node-1",
				NodeType:        "scanner-agent",
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected bootstrap error")
				}
				return
			}
			if err != nil {
				t.Fatalf("bootstrap: %v", err)
			}
			if session.SessionID != "session-1" {
				t.Fatalf("unexpected session_id: %s", session.SessionID)
			}
			if session.SessionToken != "token-1" {
				t.Fatalf("unexpected session_token: %s", session.SessionToken)
			}
		})
	}
}

func TestHTTPTransportHeartbeat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		status           int
		response         string
		wantAuth         string
		wantErr          bool
		wantRunningJobs  uint32
		wantObservedTime time.Time
		wantAttemptID    string
	}{
		{
			name:             "success",
			status:           http.StatusAccepted,
			wantAuth:         "Bearer token-1",
			wantRunningJobs:  3,
			wantObservedTime: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
			wantAttemptID:    "attempt-1",
		},
		{
			name:             "expired session",
			status:           http.StatusGone,
			response:         `{"error":"expired"}`,
			wantAuth:         "Bearer token-1",
			wantErr:          true,
			wantObservedTime: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
			wantAttemptID:    "attempt-1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/node-sessions/session-1/heartbeats" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != tt.wantAuth {
					t.Fatalf("unexpected auth header: %s", got)
				}

				var request HeartbeatRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if request.RunningJobs != tt.wantRunningJobs {
					t.Fatalf("unexpected running_jobs: %d", request.RunningJobs)
				}
				if !request.ObservedAt.Equal(tt.wantObservedTime) {
					t.Fatalf("unexpected observed_at: %s", request.ObservedAt)
				}
				if len(request.ActiveAttempts) != 1 {
					t.Fatalf("unexpected active_attempt count: %d", len(request.ActiveAttempts))
				}
				if request.ActiveAttempts[0].AttemptID != tt.wantAttemptID {
					t.Fatalf("unexpected active_attempt attempt_id: %s", request.ActiveAttempts[0].AttemptID)
				}

				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("create transport: %v", err)
			}

			err = transport.Heartbeat(context.Background(), SessionState{
				SessionID:    "session-1",
				SessionToken: "token-1",
			}, HeartbeatRequest{
				LifecycleState: "ready",
				RunningJobs:    tt.wantRunningJobs,
				ObservedAt:     tt.wantObservedTime,
				ActiveAttempts: []ActiveAttemptHeartbeat{
					{
						AttemptID:      tt.wantAttemptID,
						JobID:          "job-1",
						SubtaskID:      "subtask-1",
						Status:         "running",
						CompletedUnits: 3,
						TotalUnits:     8,
						LastActivityAt: tt.wantObservedTime,
					},
				},
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected heartbeat error")
				}
				return
			}
			if err != nil {
				t.Fatalf("heartbeat: %v", err)
			}
		})
	}
}
