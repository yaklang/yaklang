package node

import (
	"context"
	"encoding/json"
	"errors"
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
				if request.HeartbeatIntervalSeconds != 30 {
					t.Fatalf(
						"unexpected heartbeat_interval_seconds: %d",
						request.HeartbeatIntervalSeconds,
					)
				}
				if request.Hostname != "host-a" {
					t.Fatalf("unexpected hostname: %q", request.Hostname)
				}
				if request.PrimaryIP != "10.0.0.5" {
					t.Fatalf("unexpected primary_ip: %q", request.PrimaryIP)
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
				EnrollmentToken:          "enroll-1",
				NodeID:                   "node-1",
				NodeType:                 "scanner-agent",
				HeartbeatIntervalSeconds: 30,
				HostInfo: HostInfo{
					Hostname:        "host-a",
					PrimaryIP:       "10.0.0.5",
					IPAddresses:     []string{"10.0.0.5", "192.168.1.7"},
					OperatingSystem: "linux",
					Architecture:    "amd64",
				},
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
		wantInterval     uint32
	}{
		{
			name:             "success",
			status:           http.StatusAccepted,
			wantAuth:         "Bearer token-1",
			wantRunningJobs:  3,
			wantObservedTime: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
			wantAttemptID:    "attempt-1",
			wantInterval:     30,
		},
		{
			name:             "expired session",
			status:           http.StatusGone,
			response:         `{"error":"expired"}`,
			wantAuth:         "Bearer token-1",
			wantErr:          true,
			wantObservedTime: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
			wantAttemptID:    "attempt-1",
			wantInterval:     30,
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
				if request.HeartbeatIntervalSeconds != tt.wantInterval {
					t.Fatalf(
						"unexpected heartbeat_interval_seconds: %d",
						request.HeartbeatIntervalSeconds,
					)
				}
				if len(request.ActiveAttempts) != 1 {
					t.Fatalf("unexpected active_attempt count: %d", len(request.ActiveAttempts))
				}
				if request.Hostname != "host-a" {
					t.Fatalf("unexpected hostname: %q", request.Hostname)
				}
				if request.PrimaryIP != "10.0.0.5" {
					t.Fatalf("unexpected primary_ip: %q", request.PrimaryIP)
				}
				if request.OperatingSystem != "linux" {
					t.Fatalf("unexpected operating_system: %q", request.OperatingSystem)
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
				LifecycleState:           "ready",
				RunningJobs:              tt.wantRunningJobs,
				ObservedAt:               tt.wantObservedTime,
				HeartbeatIntervalSeconds: tt.wantInterval,
				HostInfo: HostInfo{
					Hostname:        "host-a",
					PrimaryIP:       "10.0.0.5",
					IPAddresses:     []string{"10.0.0.5", "192.168.1.7"},
					OperatingSystem: "linux",
					Architecture:    "amd64",
				},
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

func TestHTTPTransportShutdown(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/node-sessions/session-1/shutdown" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("unexpected auth header: %s", got)
		}

		var request ShutdownRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !request.ObservedAt.Equal(observedAt) {
			t.Fatalf("unexpected observed_at: %s", request.ObservedAt)
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("create transport: %v", err)
	}

	err = transport.Shutdown(context.Background(), SessionState{
		SessionID:    "session-1",
		SessionToken: "token-1",
	}, ShutdownRequest{ObservedAt: observedAt})
	if err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestReadHTTPErrorParsesJSONPayload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"node session is not active"}`))
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
		ObservedAt:     time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected heartbeat error")
	}

	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected HTTPStatusError, got %T", err)
	}
	if statusErr.StatusCode != http.StatusConflict {
		t.Fatalf("unexpected status code: %d", statusErr.StatusCode)
	}
	if statusErr.Message != "node session is not active" {
		t.Fatalf("unexpected status message: %s", statusErr.Message)
	}
	if !IsSessionInactiveTransportError(err) {
		t.Fatal("expected session inactive helper to match")
	}
}
