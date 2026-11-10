package aispec

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestInvalidInputDoesNotEnterInfiniteNetworkRecovery(t *testing.T) {
	globalRecovery(t, true)
	for _, cause := range []error{ErrRequestSizeLimit, &PromptOverflowError{PromptOverflowEvidence: PromptOverflowEvidence{Message: "context limit"}}, errors.New("AI provider returned HTTP 400: model does not support image input")} {
		calls := 0
		client := &recoveryClient{cfg: NewDefaultAIConfig(), call: func(*AIConfig) (string, error) { calls++; return "", cause }}
		_, err := chatWithGlobalRetry(client, "fixture", func(context.Context, time.Duration) error {
			t.Fatal("identical invalid input must not be retried")
			return nil
		})
		require.Error(t, err)
		require.Equal(t, 1, calls)
	}
}

func TestSerializedRequestByteLimitRejectsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); fmt.Fprint(w, "unexpected") }))
	defer server.Close()
	_, err := ChatBase(server.URL, "fixture", strings.Repeat("x", 1000), WithChatBase_AISamplingFromConfig(&AIConfig{RequestByteLimit: 64}), WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }))
	require.ErrorIs(t, err, ErrRequestSizeLimit)
	require.Equal(t, int32(0), calls.Load())
}
