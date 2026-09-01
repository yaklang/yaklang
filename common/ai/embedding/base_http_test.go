package embedding

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestOpenaiEmbeddingClient_Embedding(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected request target: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request target", http.StatusBadRequest)
			return
		}
		assert.Equal(t, "Bearer test-key", request.Header.Get("Authorization"))
		assert.Equal(t, "test-value", request.Header.Get("X-Test-Header"))
		assert.Contains(t, request.Header.Get("Accept-Encoding"), "gzip")

		var payload embeddingRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		assert.Equal(t, "Hello, world!", payload.Input)
		assert.Equal(t, "float", payload.EncodingFormat)
		assert.Equal(t, "test-model", payload.Model)

		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Content-Encoding", "gzip")
		compressed := gzip.NewWriter(writer)
		_, _ = io.WriteString(compressed, `{"object":"list","data":[{"index":0,"embedding":[3,4]}]}`)
		if err := compressed.Close(); err != nil {
			t.Errorf("close gzip response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	client := NewOpenaiEmbeddingClient(
		aispec.WithBaseURL(server.URL+"/v1"),
		aispec.WithAPIKey("test-key"),
		aispec.WithModel("test-model"),
		aispec.WithExtraHeaderString("X-Test-Header", "test-value"),
	)
	embedding, err := client.Embedding("Hello, world!")
	require.NoError(t, err)
	assert.InDeltaSlice(t, []float32{0.6, 0.8}, embedding, 1e-6)
	require.EqualValues(t, 1, requests.Load())
}

func TestOpenaiEmbeddingClient_ReusesConnections(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"data":[{"embedding":[1,0]}]}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)

	client := NewOpenaiEmbeddingClient(aispec.WithBaseURL(server.URL), aispec.WithModel("test-model"))
	for i := 0; i < 20; i++ {
		_, err := client.Embedding("reuse")
		require.NoError(t, err)
	}
	assert.EqualValues(t, 1, connections.Load(), "sequential embedding calls should reuse one keep-alive connection")
}

func TestOpenaiEmbeddingClient_VerifiesTLSCertificates(t *testing.T) {
	client := NewOpenaiEmbeddingClient(aispec.WithModel("test-model"))
	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	if transport.TLSClientConfig != nil {
		assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	}
}

func TestDecodeEmbeddingResponseFormats(t *testing.T) {
	tests := []struct {
		name string
		body string
		want [][]float32
	}{
		{name: "standard", body: `{"data":[{"embedding":[1,2]},{"embedding":[9,9]}],"model":"m"}`, want: [][]float32{{1, 2}}},
		{name: "object 2d", body: `{"metadata":{"nested":[1,{"skip":true}]},"data":[{"embedding":[[1,2],[3,4]]},{"embedding":[[9,9]]}]}`, want: [][]float32{{1, 2}, {3, 4}}},
		{name: "top level 1d", body: `[{"embedding":[1,2]},{"embedding":[3,4]}]`, want: [][]float32{{1, 2}, {3, 4}}},
		{name: "top level 2d", body: `[{"embedding":[[1,2],[3,4]]},{"embedding":[[9,9]]}]`, want: [][]float32{{1, 2}, {3, 4}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, apiErr, err := decodeEmbeddingResponse(strings.NewReader(test.body))
			require.NoError(t, err)
			require.Nil(t, apiErr)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestOpenaiEmbeddingClient_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = io.WriteString(writer, `{"data":null,"error":{"code":413,"message":"input is too large for this model","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(server.Close)

	client := NewOpenaiEmbeddingClient(aispec.WithBaseURL(server.URL), aispec.WithModel("test-model"))
	_, err := client.EmbeddingRaw(strings.Repeat("x", 1024))
	require.ErrorIs(t, err, ErrInputTooLarge)
}

func TestOpenaiEmbeddingClient_ContextCancellationDoesNotRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	client := NewOpenaiEmbeddingClient(aispec.WithContext(ctx), aispec.WithModel("test-model"))
	client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected transport call")
	})

	_, err := client.EmbeddingRaw("cancelled")
	require.Error(t, err)
	assert.LessOrEqual(t, calls.Load(), int32(1))
}

func TestOpenaiEmbeddingClient_RetriesTransportFailure(t *testing.T) {
	var calls atomic.Int32
	client := NewOpenaiEmbeddingClient(aispec.WithModel("test-model"))
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("temporary connection failure")
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: -1,
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader(`{"data":[{"embedding":[3,4]}]}`)),
			Request:       request,
		}, nil
	})

	vector, err := client.Embedding("retry")
	require.NoError(t, err)
	assert.InDeltaSlice(t, []float32{0.6, 0.8}, vector, 1e-6)
	assert.EqualValues(t, 2, calls.Load())
}

func TestNormalizeEmbeddingVectorsInPlace(t *testing.T) {
	vectors := [][]float32{{3, 4}, {0, 0}}
	normalizeEmbeddingVectors(vectors)
	assert.InDeltaSlice(t, []float32{0.6, 0.8}, vectors[0], 1e-6)
	var norm float64
	for _, value := range vectors[0] {
		norm += float64(value * value)
	}
	assert.InDelta(t, 1, math.Sqrt(norm), 1e-6)
	assert.Equal(t, []float32{0, 0}, vectors[1])
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
