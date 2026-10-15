package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type deliveryTestProvider struct {
	pending  []PendingJobDelivery
	accepted SessionState
	receipts []JobDeliveryReceipt
}

func (p *deliveryTestProvider) Snapshot() RuntimeStatus {
	return RuntimeStatus{LifecycleState: "ready"}
}
func (p *deliveryTestProvider) PendingJobDeliveries() []PendingJobDelivery { return p.pending }
func (p *deliveryTestProvider) AcceptJobDeliveryRecovery(s SessionState) error {
	if len(p.pending) > 0 && !s.JobDeliveryRecoverySupported {
		return fmt.Errorf("unsupported delivery recovery")
	}
	p.accepted = s
	return nil
}
func (p *deliveryTestProvider) ApplyJobDeliveryReceipts(r []JobDeliveryReceipt) error {
	p.receipts = r
	if len(r) > 0 {
		p.pending = nil
	}
	return nil
}

func TestJobDeliveryHTTPHandshakeAndProjectionReceipt(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(fmt.Sprint(supported), func(t *testing.T) {
			d := PendingJobDelivery{AttemptID: "attempt", JobID: "job", SubtaskID: "sub", CommandID: "cmd", NodeSessionID: "original-session", TerminalEventID: "attempt:succeeded"}
			provider := &deliveryTestProvider{pending: []PendingJobDelivery{d}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case bootstrapEndpointPath:
					var request BootstrapRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
					require.Equal(t, []PendingJobDelivery{d}, request.PendingJobDeliveries)
					_ = json.NewEncoder(w).Encode(map[string]any{"node_id": "node", "node_session_id": "new-session", "session_token": "fixture", "job_delivery_recovery_supported": supported, "accepted_job_delivery_attempts": []string{"attempt"}})
				case "/v1/node-sessions/new-session/heartbeats":
					require.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
					var request HeartbeatRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
					require.Equal(t, "original-session", request.PendingJobDeliveries[0].NodeSessionID)
					_ = json.NewEncoder(w).Encode(HeartbeatResponse{JobDeliveryReceipts: []JobDeliveryReceipt{{AttemptID: "attempt", TerminalEventID: d.TerminalEventID, Disposition: "projected"}}})
				default:
					w.WriteHeader(http.StatusAccepted)
				}
			}))
			defer server.Close()
			base, err := NewNodeBase(BaseConfig{BaseDir: t.TempDir(), PlatformAPIBaseURL: server.URL, EnrollmentToken: "fixture", StatusProvider: provider})
			require.NoError(t, err)
			defer base.Shutdown()
			err = base.bootstrapSession()
			if !supported {
				require.ErrorContains(t, err, "unsupported delivery recovery")
				_, ok := base.GetSessionState()
				require.False(t, ok)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "new-session", provider.accepted.SessionID)
			require.Equal(t, []string{"attempt"}, provider.accepted.AcceptedJobDeliveryAttempts)
			require.NoError(t, base.heartbeat())
			require.Empty(t, provider.pending)
			require.Len(t, provider.receipts, 1)
			_, err = base.transport.Bootstrap(context.Background(), BootstrapRequest{PendingJobDeliveries: []PendingJobDelivery{d}})
			require.NoError(t, err)
		})
	}
}
