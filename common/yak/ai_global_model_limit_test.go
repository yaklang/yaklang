package yak

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/yakscript"
	"github.com/yaklang/yaklang/common/consts"
)

func TestGlobalModelLimitMixedProvidersAndRetries(t *testing.T) {
	previous := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{ModelRequestsPerMinute: 600, RetryForever: true})
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	var active, peak atomic.Int32
	var mu sync.Mutex
	var starts []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := active.Add(1)
		peak.Store(max(peak.Load(), count))
		defer active.Add(-1)
		mu.Lock()
		starts = append(starts, time.Now())
		attempt := len(starts)
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		if attempt == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			io.WriteString(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ready\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			opts := []aispec.AIConfigOption{aispec.WithContext(ctx), aispec.WithTimeout(.1), aispec.WithBaseURL(server.URL), aispec.WithModel(fmt.Sprint(i)), aispec.WithAPIKey("fixture"), aispec.WithDisableProviderFallback(true)}
			if i%2 == 0 {
				opts = append(opts, aispec.WithType("yak-script"), aispec.WithScript(strings.NewReplacer(`mode = "json"`, `mode = "sse"`, "https://model.example/v1/chat/completions", server.URL, "timeoutSeconds = 240", "timeoutSeconds = 0.1").Replace(yakscript.ExampleScript)))
			} else {
				opts = append(opts, aispec.WithType("openai"))
			}
			answer, err := ai.Chat("local admission test", opts...)
			require.NoError(t, err)
			require.Equal(t, "ready", answer)
		}(i)
	}
	wg.Wait()
	require.Greater(t, peak.Load(), int32(1), "RPM admission must not serialize healthy in-flight requests")
	require.Len(t, starts, 5, "every retry must be a new admitted HTTP attempt")
	require.GreaterOrEqual(t, starts[4].Sub(starts[0]), time.Second, "retry obeys shared cooldown")
}
