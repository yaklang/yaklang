package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPC_Ai_List_Model(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in github actions")
	}
	client, err := NewLocalClient()
	require.NoError(t, err)
	config := make(map[string]string)
	config["api_key"] = "${api_key}"

	//TODO:  Should use baseurl mock
	config["proxy"] = "http://127.0.0.1:7890"
	config["Type"] = "openai"
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	rsp, err := client.ListAiModel(context.Background(), &ypb.ListAiModelRequest{
		Config: string(raw),
	})
	require.NoError(t, err)
	require.NotNil(t, rsp)
	for _, name := range rsp.ModelName {
		t.Log(name)
	}
}

type testAIModelClient struct {
	config   *aispec.AIConfig
	response string
}

func (c *testAIModelClient) LoadOption(opts ...aispec.AIConfigOption) {
	c.config = aispec.NewDefaultAIConfig(opts...)
}

func (c *testAIModelClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	return nil, nil
}

func (c *testAIModelClient) CheckValid() error {
	return nil
}

func (c *testAIModelClient) GetConfig() *aispec.AIConfig {
	return c.config
}

func (c *testAIModelClient) Chat(_ string, _ ...any) (string, error) {
	if c.config == nil {
		return "", utils.Error("config is nil")
	}

	time.Sleep(20 * time.Millisecond)
	if c.config.RawHTTPRequestResponseCallback != nil {
		c.config.RawHTTPRequestResponseCallback(
			[]byte("POST /v1/chat/completions HTTP/1.1\r\nHost: mock.local\r\nContent-Type: application/json\r\nAccept: application/json\r\n\r\n{\"content\":\"ping\"}"),
			[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(`{"choices":[{"message":{"content":"mock-response"}}]}`),
		)
	}
	if c.config.ReasonStreamHandler != nil {
		c.config.ReasonStreamHandler(strings.NewReader("thinking"))
	}
	if c.config.StreamHandler != nil {
		c.config.StreamHandler(strings.NewReader(c.response))
	}
	time.Sleep(20 * time.Millisecond)
	return c.response, nil
}

func (c *testAIModelClient) ChatStream(_ string) (io.Reader, error) {
	return strings.NewReader(c.response), nil
}

func (c *testAIModelClient) ExtractData(_ string, _ string, _ map[string]any) (map[string]any, error) {
	return nil, utils.Error("unsupported")
}

func (c *testAIModelClient) SupportedStructuredStream() bool {
	return false
}

func (c *testAIModelClient) StructuredStream(_ string, _ ...any) (chan *aispec.StructuredData, error) {
	return nil, utils.Error("unsupported")
}

func (c *testAIModelClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return nil, nil
}

func TestGRPC_Ai_Config_Health_Check(t *testing.T) {
	const providerType = "grpc-ai-model-test-provider"
	aispec.Register(providerType, func() aispec.AIClient {
		return &testAIModelClient{
			response: "mock-response",
		}
	})

	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)

	rsp, err := client.AIConfigHealthCheck(context.Background(), &ypb.AIConfigHealthCheckRequest{
		Config: &ypb.ThirdPartyApplicationConfig{
			Type:   providerType,
			APIKey: "test-key",
			ExtraParams: []*ypb.KVPair{
				{Key: "model", Value: "mock-model"},
			},
		},
		Content: "ping",
	})
	require.NoError(t, err)
	require.NotNil(t, rsp)
	assert.GreaterOrEqual(t, rsp.GetFirstByteCostMs(), int64(1))
	assert.GreaterOrEqual(t, rsp.GetTotalCostMs(), rsp.GetFirstByteCostMs())
	assert.Contains(t, rsp.GetRawRequest(), "POST /v1/chat/completions HTTP/1.1")
	assert.Contains(t, rsp.GetRawResponse(), "HTTP/1.1 200 OK")
	assert.Contains(t, rsp.GetRawResponse(), `"mock-response"`)
	assert.Equal(t, int32(200), rsp.GetResponseStatusCode())
	assert.Equal(t, "mock-response", rsp.GetResponseContent())
	assert.Empty(t, rsp.GetErrorMessage())
}

func TestRecommendAIHealthCheckConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:    "openrouter",
		APIKey:  "test-key",
		BaseURL: server.URL + "/bad",
		ExtraParams: []*ypb.KVPair{
			{Key: "model", Value: "mock-model"},
		},
	}

	initial := executeAIConfigHealthCheck(context.Background(), cfg, "ping")
	assert.NotEmpty(t, initial.GetErrorMessage())

	recommend := recommendAIHealthCheckConfig(context.Background(), cfg, "ping")
	require.NotNil(t, recommend)
	assert.Empty(t, recommend.GetBaseURL())
	assert.Equal(t, server.URL+"/api/v1/chat/completions", recommend.GetEndpoint())
	assert.True(t, recommend.GetEnableEndpoint())
	assert.Empty(t, recommend.GetProxy())
	assert.True(t, recommend.GetNoHttps())
}

func TestBuildRecommendedAIConfigs_TogglesProxyAndSuffixes(t *testing.T) {
	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:    "openrouter",
		APIKey:  "test-key",
		BaseURL: "http://mock.local",
		Proxy:   "http://127.0.0.1:8080",
		ExtraParams: []*ypb.KVPair{
			{Key: "model", Value: "mock-model"},
		},
	}

	candidates := buildRecommendedAIConfigs(cfg)
	require.NotEmpty(t, candidates)

	found := false
	for _, candidate := range candidates {
		if candidate.GetEndpoint() == "http://mock.local/api/v1/chat/completions" && candidate.GetProxy() == "" && candidate.GetEnableEndpoint() {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestBuildRecommendedAIConfigs_UsesResponsesSuffixesByAPIType(t *testing.T) {
	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:    "openai",
		APIKey:  "test-key",
		BaseURL: "https://mock.local",
		APIType: "responses",
		ExtraParams: []*ypb.KVPair{
			{Key: "model", Value: "mock-model"},
		},
	}

	candidates := buildRecommendedAIConfigs(cfg)
	require.NotEmpty(t, candidates)

	var foundResponses bool
	for _, candidate := range candidates {
		if candidate.GetAPIType() == "responses" && candidate.GetEndpoint() == "https://mock.local/v1/responses" && candidate.GetEnableEndpoint() {
			foundResponses = true
		}
	}
	assert.True(t, foundResponses)
}

func TestBuildRecommendedAIConfigs_IncludesAlternativeAPITypeCandidates(t *testing.T) {
	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:    "openai",
		APIKey:  "test-key",
		BaseURL: "https://mock.local",
		APIType: "responses",
		ExtraParams: []*ypb.KVPair{
			{Key: "model", Value: "mock-model"},
		},
	}

	candidates := buildRecommendedAIConfigs(cfg)
	require.NotEmpty(t, candidates)

	var foundResponses bool
	var foundChatCompletions bool
	for _, candidate := range candidates {
		if candidate.GetEndpoint() == "https://mock.local/v1/responses" && candidate.GetAPIType() == "responses" && candidate.GetEnableEndpoint() {
			foundResponses = true
		}
		if candidate.GetEndpoint() == "https://mock.local/v1/chat/completions" && candidate.GetAPIType() == "chat_completions" && candidate.GetEnableEndpoint() {
			foundChatCompletions = true
		}
	}
	assert.True(t, foundResponses)
	assert.True(t, foundChatCompletions)
}

func TestRecommendAIHealthCheckConfig_CanCorrectAPIType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"output_text":"ok"}`))
	}))
	defer server.Close()

	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:    "openai",
		APIKey:  "test-key",
		BaseURL: server.URL,
		APIType: "chat_completions",
		ExtraParams: []*ypb.KVPair{
			{Key: "model", Value: "mock-model"},
		},
	}

	initial := executeAIConfigHealthCheck(context.Background(), cfg, "ping")
	assert.NotEmpty(t, initial.GetErrorMessage())

	recommend := recommendAIHealthCheckConfig(context.Background(), cfg, "ping")
	require.NotNil(t, recommend)
	assert.Empty(t, recommend.GetBaseURL())
	assert.Equal(t, server.URL+"/v1/responses", recommend.GetEndpoint())
	assert.True(t, recommend.GetEnableEndpoint())
	assert.Equal(t, "responses", recommend.GetAPIType())
	assert.True(t, recommend.GetNoHttps())
}

func TestRecommendAIHealthCheckConfig_FallbackToEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/openai/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:    "openai",
		APIKey:  "test-key",
		BaseURL: server.URL + "/custom/openai/chat/completions",
		ExtraParams: []*ypb.KVPair{
			{Key: "model", Value: "mock-model"},
		},
	}

	recommend := recommendAIHealthCheckConfig(context.Background(), cfg, "ping")
	require.NotNil(t, recommend)
	assert.Empty(t, recommend.GetBaseURL())
	assert.Equal(t, server.URL+"/custom/openai/chat/completions", recommend.GetEndpoint())
	assert.True(t, recommend.GetEnableEndpoint())
	assert.True(t, recommend.GetNoHttps())
}
