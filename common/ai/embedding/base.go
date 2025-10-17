package embedding

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type OpenaiEmbeddingClient struct {
	config *aispec.AIConfig
}

var _ aispec.EmbeddingCaller = (*OpenaiEmbeddingClient)(nil)

func NewOpenaiEmbeddingClient(opt ...aispec.AIConfigOption) *OpenaiEmbeddingClient {
	config := aispec.NewDefaultAIConfig(opt...)
	c := &OpenaiEmbeddingClient{
		config: config,
	}
	return c
}

type embeddingRequest struct {
	Input          string `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Model          string `json:"model,omitempty"`
}

type embeddingItem struct {
	Index     int         `json:"index"`
	Embedding [][]float32 `json:"embedding"`
}

type embeddingResponse []embeddingItem

// 错误响应结构体
type errorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// 预定义的错误类型
var (
	ErrInputTooLarge = utils.Error("input is too large")
)

func (c *OpenaiEmbeddingClient) Embedding(text string) ([]float32, error) {
	// Prepare the request
	req := embeddingRequest{
		Input:          text,
		EncodingFormat: "float",
	}

	if c.config.Model != "" {
		req.Model = c.config.Model
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, utils.Errorf("marshal request data failed: %v", err)
	}

	var targetUrl string
	if c.config.BaseURL != "" {
		targetUrl, err = url.JoinPath(c.config.BaseURL, "/embeddings")
		if err != nil {
			targetUrl = c.config.BaseURL + "/embeddings"
		}
	} else if c.config.Domain != "" {
		if c.config.NoHttps {
			targetUrl = fmt.Sprintf("http://%s/embeddings", c.config.Domain)
		} else {
			targetUrl = fmt.Sprintf("https://%s/embeddings", c.config.Domain)
		}
	} else {
		targetUrl = "http://127.0.0.1:8080/embeddings"
	}

	var pocOpts []poc.PocConfigOption

	if c.config.APIKey != "" {
		pocOpts = append(pocOpts, poc.WithAppendHeader("Authorization", "Bearer "+c.config.APIKey))
	}

	// Add timeout
	if c.config.Timeout > 0 {
		pocOpts = append(pocOpts, poc.WithTimeout(c.config.Timeout))
	}

	// Add proxy if configured
	if c.config.Proxy != "" {
		pocOpts = append(pocOpts, poc.WithProxy(c.config.Proxy))
	}

	// Add context if available
	if c.config.Context != nil {
		pocOpts = append(pocOpts, poc.WithContext(c.config.Context))
	}

	rspInst, _, err := poc.DoPOST(targetUrl,
		append(pocOpts,
			poc.WithBody(string(jsonData)),
			poc.WithAppendHeader("Content-Type", "application/json; charset=UTF-8"),
			poc.WithAppendHeader("Accept", "application/json"),
		)...,
	)

	if err != nil {
		return nil, utils.Errorf("request embeddings failed: %v", err)
	}

	// Get response body
	body := lowhttp.GetHTTPPacketBody(rspInst.RawPacket)

	// 首先尝试按正确响应解析
	var response embeddingResponse
	if err := json.Unmarshal(body, &response); err == nil && len(response) > 0 && len(response[0].Embedding) > 0 {
		// 成功解析为正确响应
		last := utils.GetLastElement(response[0].Embedding)
		last = NormalizeVector(last, 2, 1e-6)
		return last, nil
	}

	// 如果正确响应解析失败，尝试解析错误响应
	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		// 检查是否包含 "input is too large" 错误
		if strings.Contains(strings.ToLower(errResp.Error.Message), "input is too large") {
			return nil, ErrInputTooLarge
		}
		// 返回其他API错误
		return nil, utils.Errorf("API error: %s (code: %d, type: %s)",
			errResp.Error.Message, errResp.Error.Code, errResp.Error.Type)
	}

	// 如果两种格式都解析失败，返回通用错误
	return nil, utils.Errorf("failed to parse response: %s", string(body))
}
