package vectorstore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
)

func TestLocalEmbeddingProbeBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Stalling after connection must share the same budget as dialing/retries.
		select {
		case <-r.Context().Done():
		case <-time.After(4 * time.Second):
		}
	}))
	defer server.Close()
	started := time.Now()
	_, err := probeLocalEmbedding(context.Background(), strings.TrimPrefix(server.URL, "http://"), "probe")
	require.ErrorContains(t, err, "budget 3s")
	require.Less(t, time.Since(started), 3500*time.Millisecond)
	require.Greater(t, time.Since(started), 2800*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started = time.Now()
	_, err = probeLocalEmbedding(ctx, strings.TrimPrefix(server.URL, "http://"), "probe")
	require.Error(t, err)
	require.Less(t, time.Since(started), 500*time.Millisecond, "caller cancellation must shorten the probe budget")
}

func TestLocalEmbeddingProbeReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/embeddings", r.URL.Path)
		fmt.Fprint(w, `{"data":[{"embedding":[0.1,0.2,0.3]}]}`)
	}))
	defer server.Close()
	vector, err := probeLocalEmbedding(context.Background(), strings.TrimPrefix(server.URL, "http://"), "probe")
	require.NoError(t, err)
	require.Len(t, vector, 3)
}

func TestEmbeddingCachedClientsDoNotReprobe(t *testing.T) {
	ResetAIBalanceFreeService()
	ResetService()
	t.Cleanup(func() { ResetAIBalanceFreeService(); ResetService() })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"data":[{"embedding":[0.1,0.2,0.3]}]}`)
	}))
	defer server.Close()
	online := &AIBalanceFreeEmbedding{embedding: embedding.NewOpenaiEmbeddingClient(aispec.WithBaseURL(server.URL)), available: true, modelName: normalizedModelName}
	aibalanceFreeOnce.Do(func() {
		aibalanceFreeInstance, aibalanceFreeAvailable, aibalanceFreeCheckDone = online, true, true
	})
	// Cache a local client whose endpoint is deliberately not running. Fetching
	// it again must be immediate rather than waiting 30s for a new TCP probe.
	local := NewLocalModelEmbedding(nil, "127.0.0.1:1")
	embeddingServiceOnce.Do(func() { embeddingServiceInstance = local })
	for i := 0; i < 3; i++ {
		dimension, err := CheckOnlineEmbedding()
		require.NoError(t, err)
		require.Equal(t, normalizedModelDimension, dimension)
		cfg := &CollectionConfig{}
		require.NoError(t, cfg.FixEmbeddingClient())
		require.Same(t, online, cfg.EmbeddingClient, "AIBalance must win over a cached local client")
		started := time.Now()
		cached, err := GetLocalEmbeddingService()
		require.NoError(t, err)
		require.Same(t, local, cached)
		require.Less(t, time.Since(started), 100*time.Millisecond)
	}
	require.Zero(t, requests, "cached checks and client selection must not send extra embeddings")
	_, err := Embedding("actual input")
	require.NoError(t, err)
	require.Equal(t, 1, requests, "generic embedding must use AIBalance for real work without an extra probe")
}
