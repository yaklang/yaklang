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
	//log.Infof("EmbeddingRaw called with text length: %d, model: %s", len(text), c.config.Model)
	//log.Infof("EmbeddingRaw config: BaseURL=%s, Domain=%s, NoHttps=%v",
	//	c.config.BaseURL, c.config.Domain, c.config.NoHttps)

	// Prepare the request
	req := embeddingRequest{
		Input:          text,
		EncodingFormat: "float",
	}

	if c.config.Model != "" {
		req.Model = c.config.Model
		//log.Infof("EmbeddingRaw: Using model from config: %s", c.config.Model)
	} else {
		log.Warnf("EmbeddingRaw: No model specified in config!")
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, utils.Errorf("marshal request data failed: %v", err)
	}
	//log.Infof("Embedding request body: %s", string(jsonData))

	var targetUrl string
	if c.config.BaseURL != "" {
		targetUrl, err = url.JoinPath(c.config.BaseURL, "/embeddings")
		if err != nil {
			targetUrl = c.config.BaseURL + "/embeddings"
		}
		//log.Infof("Embedding URL (from BaseURL): %s (BaseURL: %s)", targetUrl, c.config.BaseURL)
	} else if c.config.Domain != "" {
		if c.config.NoHttps {
			targetUrl = fmt.Sprintf("http://%s/embeddings", c.config.Domain)
		} else {
			targetUrl = fmt.Sprintf("https://%s/embeddings", c.config.Domain)
		}
		//log.Infof("Embedding URL (from Domain): %s (Domain: %s, NoHttps: %v)", targetUrl, c.config.Domain, c.config.NoHttps)
	} else {
		targetUrl = "http://127.0.0.1:8080/embeddings"
		//log.Infof("Embedding URL (default): %s", targetUrl)
	}

	var pocOpts []poc.PocConfigOption

	if c.config.APIKey != "" {
		pocOpts = append(pocOpts, poc.WithAppendHeader("Authorization", "Bearer "+c.config.APIKey))
		//log.Infof("Embedding request: Using API Key: %s", utils.ShrinkString(c.config.APIKey, 8))
	} else {
		//log.Warnf("Embedding request: No API Key provided!")
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

	//log.Infof("Embedding request: Sending POST to %s", targetUrl)
	//log.Infof("Embedding request headers: Content-Type=application/json, Authorization=Bearer %s...",
	//	utils.ShrinkString(c.config.APIKey, 4))

	rspInst, _, err := poc.DoPOST(targetUrl,
		append(pocOpts,
			poc.WithBody(string(jsonData)),
			poc.WithAppendHeader("Content-Type", "application/json; charset=UTF-8"),
			poc.WithAppendHeader("Accept", "application/json"),
			poc.WithSave(false),    // do not save embedding requests to database
			poc.WithConnPool(true), // enable connection pool for better performance
		)...,
	)

	if err != nil {
		log.Errorf("Embedding request failed: %v", err)
		return nil, utils.Errorf("request embeddings failed: %v", err)
	}

	//log.Infof("Embedding request completed, parsing response...")

	// Get response body
	body := lowhttp.GetHTTPPacketBody(rspInst.RawPacket)
	statusCode := lowhttp.ExtractStatusCodeFromResponse(rspInst.RawPacket)
	if statusCode >= 400 {
		log.Warnf("Embedding response error body: %s", utils.ShrinkString(string(body), 500))
	} else {
		if statusCode != 200 {
			log.Infof("Embedding response status: %d, body length: %d", statusCode, len(body))
		}
	}

	// 策略1: 首先尝试解析标准格式（一维向量 []float32）
	var response embeddingResponse
	if err := json.Unmarshal(body, &response); err == nil && len(response.Data) > 0 && len(response.Data[0].Embedding) > 0 {
		// 成功解析为标准响应格式（一维向量）
		// 将单个向量包装成二维数组返回
		embedding := response.Data[0].Embedding
		embedding = NormalizeVector(embedding, 2, 1e-6)
		//log.Infof("Successfully parsed embedding response as 1D format ([]float32), dimension: %d", len(embedding))
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

		//log.Infof("Successfully parsed embedding response as 2D format ([][]float32), vector count: %d, first vector dimension: %d",
		//	vectorCount, len(normalizedVectors[0]))
		return normalizedVectors, nil
	}

	var response3 []embeddingItem2D
	if err := json.Unmarshal(body, &response3); err == nil && len(response3) > 0 && len(response3[0].Embedding) > 0 {
		// 成功解析为二维向量格式，返回所有向量
		embeddingVectors := response3[0].Embedding
		vectorCount := len(embeddingVectors)

		// 对所有向量进行归一化处理
		normalizedVectors := make([][]float32, vectorCount)
		for i, vec := range embeddingVectors {
			if len(vec) > 0 {
				normalizedVectors[i] = NormalizeVector(vec, 2, 1e-6)
			}
		}

		//log.Infof("Successfully parsed embedding response as 2D format ([][]float32), vector count: %d, first vector dimension: %d",
		//	vectorCount, len(normalizedVectors[0]))
		return normalizedVectors, nil
	}

	var response4 []embeddingItem
	if err := json.Unmarshal(body, &response4); err == nil && len(response4) > 0 && len(response4[0].Embedding) > 0 {
		// 成功解析为一维向量格式，返回所有向量
		embeddingVectors := response4
		vectorCount := len(embeddingVectors)
		normalizedVectors := make([][]float32, vectorCount)
		for i, item := range embeddingVectors {
			if len(item.Embedding) > 0 {
				normalizedVectors[i] = NormalizeVector(item.Embedding, 2, 1e-6)
			}
		}
		//log.Infof("Successfully parsed embedding response as 1D array format ([]embeddingItem), vector count: %d, first vector dimension: %d",
		//	vectorCount, len(normalizedVectors[0]))
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
	fmt.Println(string(body))
	if len(bodyStr) > 500 {
		bodyStr = bodyStr[:500] + "... (truncated)"
	}
	return nil, utils.Errorf("failed to parse embedding response in any known format: %s", bodyStr)
}

// Embedding 返回单个向量（保持向后兼容）
// 如果服务器返回多个向量，返回最后一个（使用 last 池化方法）
func (c *OpenaiEmbeddingClient) Embedding(text string) ([]float32, error) {
	vectors, err := c.EmbeddingRaw(text)
	if err != nil {
		return nil, err
	}

	if len(vectors) == 0 {
		return nil, utils.Error("no embedding vectors returned")
	}

	if len(vectors) > 1 {
		log.Infof("Server returned %d embedding vectors, using last pooling method (returning last vector)", len(vectors))
	}

	// 使用 last 池化方法：返回最后一个向量
	return vectors[len(vectors)-1], nil
}
