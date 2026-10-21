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
