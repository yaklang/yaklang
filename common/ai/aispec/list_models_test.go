package aispec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func TestListChatModels(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantModels []string
		wantError  string
	}{
		{
			name:       "available models",
			statusCode: http.StatusOK,
			body:       `{"object":"list","data":[{"id":"test-model"},{"id":"another-model"}]}`,
			wantModels: []string{"test-model", "another-model"},
		},
		{name: "empty model list", statusCode: http.StatusOK, body: `{"object":"list","data":[]}`},
		{name: "nginx 404", statusCode: http.StatusNotFound, body: `<html><body>404 Not Found</body></html>`},
		{name: "method not allowed", statusCode: http.StatusMethodNotAllowed, body: `{"error":"method not allowed"}`},
		{name: "not implemented", statusCode: http.StatusNotImplemented},
		{name: "bad request", statusCode: http.StatusBadRequest, body: `{"error":"bad request"}`, wantError: "HTTP 400"},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, body: `{"error":"invalid API key"}`, wantError: "HTTP 401: "},
		{name: "forbidden", statusCode: http.StatusForbidden, body: `{"error":"permission denied"}`, wantError: "HTTP 403: "},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, body: `{"error":"rate limited"}`, wantError: "HTTP 429: "},
		{name: "server error with list body", statusCode: http.StatusInternalServerError, body: `{"data":[{"id":"invalid"}]}`, wantError: "HTTP 500: "},
		{name: "bad gateway", statusCode: http.StatusBadGateway, body: "bad gateway", wantError: "HTTP 502: bad gateway"},
		{name: "empty successful response", statusCode: http.StatusOK, wantError: "empty response"},
		{name: "malformed successful response", statusCode: http.StatusOK, body: "not JSON", wantError: "unmarshal models failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/openapi/compatible-mode/v1/models" {
					t.Errorf("unexpected model discovery request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			models, err := ListChatModels(server.URL+"/openapi/compatible-mode/v1/chat/completions", func() ([]poc.PocConfigOption, error) {
				return nil, nil
			})
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				require.Nil(t, models)
				return
			}
			require.NoError(t, err)
			var names []string
			for _, model := range models {
				names = append(names, model.Id)
			}
			require.Equal(t, tt.wantModels, names)
		})
	}
}

func TestListChatModels_CanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("canceled discovery should not reach the provider")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	models, err := ListChatModels(server.URL+"/v1/chat/completions", func() ([]poc.PocConfigOption, error) {
		return []poc.PocConfigOption{poc.WithContext(ctx)}, nil
	})
	require.Error(t, err, "request failures must not become empty model lists")
	require.Nil(t, models)
}
