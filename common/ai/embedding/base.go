package embedding

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
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

// embeddingItem 一维向量格式（标准 OpenAI 格式）
type embeddingItem struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// embeddingItem2D 二维向量格式（某些服务商的格式）
type embeddingItem2D struct {
	Index     int         `json:"index"`
	Embedding [][]float32 `json:"embedding"`
}

// embeddingResponse 标准响应格式
type embeddingResponse struct {
	Object string          `json:"object"`
	Data   []embeddingItem `json:"data"`
	Model  string          `json:"model"`
}

// embeddingResponse2D 二维向量响应格式
type embeddingResponse2D struct {
	Object string            `json:"object"`
	Data   []embeddingItem2D `json:"data"`
	Model  string            `json:"model"`
}

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

// EmbeddingRaw 返回原始的 embedding 结果，保留服务器返回的所有向量
func (c *OpenaiEmbeddingClient) EmbeddingRaw(text string) ([][]float32, error) {
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
			poc.WithSave(false), // do not save embedding requests to database
		)...,
	)

	if err != nil {
		return nil, utils.Errorf("request embeddings failed: %v", err)
	}

	// Get response body
	body := lowhttp.GetHTTPPacketBody(rspInst.RawPacket)

	// 策略1: 首先尝试解析标准格式（一维向量 []float32）
	var response embeddingResponse
	if err := json.Unmarshal(body, &response); err == nil && len(response.Data) > 0 && len(response.Data[0].Embedding) > 0 {
		// 成功解析为标准响应格式（一维向量）
		// 将单个向量包装成二维数组返回
		embedding := response.Data[0].Embedding
		embedding = NormalizeVector(embedding, 2, 1e-6)
		log.Infof("Successfully parsed embedding response as 1D format ([]float32), dimension: %d", len(embedding))
		return [][]float32{embedding}, nil
	}

	// 策略2: 尝试解析二维向量格式（[][]float32）
	var response2D embeddingResponse2D
	if err := json.Unmarshal(body, &response2D); err == nil && len(response2D.Data) > 0 && len(response2D.Data[0].Embedding) > 0 {
		// 成功解析为二维向量格式，返回所有向量
		embeddingVectors := response2D.Data[0].Embedding
		vectorCount := len(embeddingVectors)

		// 对所有向量进行归一化处理
		normalizedVectors := make([][]float32, vectorCount)
		for i, vec := range embeddingVectors {
			if len(vec) > 0 {
				normalizedVectors[i] = NormalizeVector(vec, 2, 1e-6)
			}
		}

		log.Infof("Successfully parsed embedding response as 2D format ([][]float32), vector count: %d, first vector dimension: %d",
			vectorCount, len(normalizedVectors[0]))
		return normalizedVectors, nil
	}

	// 策略3: 尝试解析错误响应
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

	// 如果所有格式都解析失败，返回通用错误
	// 截断响应体以避免日志过长
	bodyStr := string(body)
	if len(bodyStr) > 500 {
		bodyStr = bodyStr[:500] + "... (truncated)"
	}
	return nil, utils.Errorf("failed to parse embedding response in any known format: %s", bodyStr)
}

// Embedding 返回单个向量（保持向后兼容）
// 如果服务器返回多个向量，只返回第一个并记录警告
func (c *OpenaiEmbeddingClient) Embedding(text string) ([]float32, error) {
	vectors, err := c.EmbeddingRaw(text)
	if err != nil {
		return nil, err
	}

	if len(vectors) == 0 {
		return nil, utils.Error("no embedding vectors returned")
	}

	if len(vectors) > 1 {
		log.Warnf("Server returned %d embedding vectors, but Embedding() only returns the first one. Use EmbeddingRaw() to get all vectors.", len(vectors))
	}

	return vectors[0], nil
}
