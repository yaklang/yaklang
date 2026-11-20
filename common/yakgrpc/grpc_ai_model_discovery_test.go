package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
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

func TestListAiModelCachesImageMetadataForLegacySave(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected discovery request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"deepseek-flash","input_modalities":["text","image"]},{"id":"text-only","input_modalities":["text"]},{"id":"unknown"},{"id":"nested","architecture":{"input_modalities":["text","image"]}}]}`)
	}))
	defer server.Close()
	ctx := context.Background()
	service := &Server{}
	baseURL := server.URL + "/v1/chat/completions"
	config, err := json.Marshal(map[string]string{"Type": "deepseek", "api_key": "fixture", "base_url": baseURL})
	require.NoError(t, err)
	models, err := service.ListAiModel(ctx, &ypb.ListAiModelRequest{Config: string(config)})
	require.NoError(t, err)
	require.Equal(t, []string{"deepseek-flash", "text-only", "unknown", "nested"}, models.GetModelName())

	provider := &ypb.ThirdPartyApplicationConfig{Type: "deepseek", APIKey: "fixture", BaseURL: baseURL}
	for name, want := range map[string]*bool{
		"deepseek-flash": proto.Bool(true), "text-only": proto.Bool(false), "unknown": nil, "nested": proto.Bool(true),
	} {
		model := &ypb.AIModelConfig{ModelName: name, Provider: provider}
		require.Equal(t, want, ai.DiscoverModelImageInput(ctx, model), name)
	}
	require.EqualValues(t, 1, requests.Load())

	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&schema.GeneralStorage{}).Error)
	legacy := &ypb.AIGlobalConfig{IntelligentModels: []*ypb.AIModelConfig{{ModelName: "deepseek-flash", Provider: provider}}}
	service.profileDatabase = db
	previous := yakit.GetCachedAIGlobalConfig()
	t.Cleanup(func() { _ = yakit.ApplyAIGlobalConfig(nil, previous) })
	_, err = service.SetAIGlobalConfig(ctx, legacy)
	require.NoError(t, err)
	saved, err := service.GetAIGlobalConfig(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, proto.Bool(true), saved.IntelligentModels[0].SupportsImageInput)
	require.Nil(t, saved.IntelligentModels[0].SupportsImageToolCalling)
	require.EqualValues(t, 1, requests.Load(), "saving reuses the same provider's catalog")
}
