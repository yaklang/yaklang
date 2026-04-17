package aihttp_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

const aiHTTPHealthCheckToolName = "ai_config_health_check"

type fakeHealthCheckAIClient struct {
	config *aispec.AIConfig
}

func (c *fakeHealthCheckAIClient) Chat(string, ...any) (string, error) {
	if c.config != nil {
		if c.config.StreamHandler != nil {
			c.config.StreamHandler(strings.NewReader("ok"))
		}
		if c.config.RawHTTPRequestResponseCallback != nil {
			c.config.RawHTTPRequestResponseCallback(
				[]byte("POST /v1/chat/completions HTTP/1.1\r\n\r\n"),
				[]byte("HTTP/1.1 200 OK\r\n"),
				[]byte(`{"ok":true}`),
			)
		}
	}
	return `{"@action":"call-tool","tool":"` + aiHTTPHealthCheckToolName + `","identifier":"health_check","params":{"content":"ping","summary":"healthy"}}`, nil
}

func (c *fakeHealthCheckAIClient) ChatStream(string) (io.Reader, error) {
	return strings.NewReader(""), nil
}

func (c *fakeHealthCheckAIClient) ExtractData(string, string, map[string]any) (map[string]any, error) {
	return nil, nil
}

func (c *fakeHealthCheckAIClient) LoadOption(opts ...aispec.AIConfigOption) {
	c.config = aispec.NewDefaultAIConfig(opts...)
}

func (c *fakeHealthCheckAIClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	return nil, nil
}

func (c *fakeHealthCheckAIClient) CheckValid() error {
	return nil
}

func (c *fakeHealthCheckAIClient) GetConfig() *aispec.AIConfig {
	if c.config == nil {
		c.config = aispec.NewDefaultAIConfig()
	}
	return c.config
}

func (c *fakeHealthCheckAIClient) SupportedStructuredStream() bool {
	return false
}

func (c *fakeHealthCheckAIClient) StructuredStream(string, ...any) (chan *aispec.StructuredData, error) {
	return nil, nil
}

func (c *fakeHealthCheckAIClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return nil, nil
}

func TestAIConfigHealthCheck(t *testing.T) {
	const providerType = "aihttp-healthcheck-test-provider"
	aispec.Register(providerType, func() aispec.AIClient {
		return &fakeHealthCheckAIClient{}
	})

	gw := newTestGateway(t)

	body := []byte(`{
		"Config": {
			"Type": "` + providerType + `"
		},
		"Content": "ping"
	}`)
	req := httptest.NewRequest("POST", "/agent/setting/aiconfig/healthcheck", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := performRequest(gw, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.AIConfigHealthCheckResponse
	require.NoError(t, protojson.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.GetSuccess())
	require.Equal(t, int32(200), resp.GetResponseStatusCode())
	require.Contains(t, resp.GetResponseContent(), aiHTTPHealthCheckToolName)
	require.Contains(t, resp.GetRawRequest(), "POST /v1/chat/completions HTTP/1.1")
	require.Empty(t, resp.GetErrorMessage())
}

func TestUploadEndpointRemoved(t *testing.T) {
	gw := newTestGateway(t)

	req := httptest.NewRequest("POST", "/agent/upload", nil)
	w := performRequest(gw, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}
