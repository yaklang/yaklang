package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
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
	client, err := NewLocalClient()
	require.NoError(t, err)
	config := make(map[string]string)
	config["api_key"] = "${api_key}"
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

func TestGRPC_Ai_Test_Model(t *testing.T) {
	const providerType = "grpc-ai-model-test-provider"
	aispec.Register(providerType, func() aispec.AIClient {
		return &testAIModelClient{
			response: "mock-response",
		}
	})

	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)

	rsp, err := client.TestAIModel(context.Background(), &ypb.TestAIModelRequest{
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
	assert.Equal(t, int32(200), rsp.GetResponseStatusCode())
	assert.Equal(t, "mock-response", rsp.GetResponseContent())
	assert.Empty(t, rsp.GetErrorMessage())
}
