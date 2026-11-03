package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestListAiModel_UnsupportedDiscoveryDoesNotBlockHealthCheck(t *testing.T) {
	const modelName = "configured-model"
	const chatPath = "/openapi/compatible-mode/v1/chat/completions"
	chatRequests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/openapi/compatible-mode/v1/models":
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, "<html><body>404 Not Found</body></html>")
		case r.Method == http.MethodPost && r.URL.Path == chatPath:
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode chat request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			select {
			case chatRequests <- request:
			default:
				t.Error("unexpected extra chat request")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"@action":"call-tool","tool":"ai_config_health_check","identifier":"health_check","params":{"content":"ping","summary":"ok"}}`,
					},
					"finish_reason": "stop",
				}},
			})
		default:
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	config, err := json.Marshal(map[string]string{
		"Type":     "openai",
		"api_key":  "test-key",
		"base_url": server.URL + chatPath,
	})
	require.NoError(t, err)
	service := &Server{}
	models, err := service.ListAiModel(context.Background(), &ypb.ListAiModelRequest{Config: string(config)})
	require.NoError(t, err)
	require.Empty(t, models.GetModelName(), "unsupported discovery must not invent available models")
	require.Empty(t, chatRequests, "model discovery must not invoke a model")

	// Continue diagnostics using the configured model, even though this provider
	// implements chat/completions without implementing model discovery.
	resp, err := service.AIConfigHealthCheck(context.Background(), &ypb.AIConfigHealthCheckRequest{
		Config: &ypb.ThirdPartyApplicationConfig{
			Type:    "openai",
			APIKey:  "test-key",
			BaseURL: server.URL + chatPath,
			ExtraParams: []*ypb.KVPair{
				{Key: "model", Value: modelName},
			},
		},
		Content: "ping",
	})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess(), "%s", resp.GetErrorMessage())
	require.Equal(t, int32(http.StatusOK), resp.GetResponseStatusCode())
	select {
	case request := <-chatRequests:
		require.Equal(t, modelName, request["model"])
	default:
		t.Fatal("health check did not reach the configured chat endpoint")
	}
}
