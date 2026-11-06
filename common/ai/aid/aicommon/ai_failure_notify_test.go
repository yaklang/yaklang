package aicommon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAITransactionFailureNotifyClassification(t *testing.T) {
	tests := []struct {
		name       string
		statusLine string
		body       string
		err        error
		wantNodeID string
	}{
		{
			name:       "network",
			err:        errors.New("dial tcp: connection refused"),
			wantNodeID: notifyTypeNetworkError,
		},
		{
			name:       "quota",
			statusLine: "HTTP/1.1 402 Payment Required\r\nContent-Type: application/json\r\n\r\n",
			body:       `{"error":{"message":"insufficient balance"}}`,
			err:        errors.New("payment required"),
			wantNodeID: notify429TypeQuotaExceeded,
		},
		{
			name:       "authentication",
			statusLine: "HTTP/1.1 401 Unauthorized\r\nContent-Type: application/json\r\n\r\n",
			body:       `{"error":{"message":"invalid api key"}}`,
			err:        errors.New("unauthorized"),
			wantNodeID: notifyTypeAuthenticationFailed,
		},
		{
			name:       "generic api",
			statusLine: "HTTP/1.1 500 Internal Server Error\r\nContent-Type: application/json\r\n\r\n",
			body:       `{"error":{"message":"internal error"}}`,
			err:        errors.New("provider failed"),
			wantNodeID: notifyTypeAPIError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, snapshot := newTestConfigForHandle429WithEvents(context.Background())
			var rsp *AIResponse
			if test.statusLine != "" {
				rsp = NewUnboundAIResponse()
				rsp.SetRawHTTPResponseData([]byte(test.statusLine), []byte(test.body))
			}
			emitAITransactionFailureNotify(cfg, rsp, test.err)
			events := snapshot()
			require.NotEmpty(t, events)
			require.Equal(t, test.wantNodeID, events[len(events)-1].NodeId)
		})
	}
}
