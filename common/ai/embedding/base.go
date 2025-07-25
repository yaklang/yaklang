package embedding

import (
	"encoding/json"
	"fmt"
	"net/url"

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

	// Parse the response
	var response embeddingResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, utils.Errorf("unmarshal response failed: %v", err)
	}

	// Check if we have embeddings
	if len(response) == 0 {
		return nil, utils.Errorf("no embedding data returned")
	}
	if len(response[0].Embedding) == 0 {
		return nil, utils.Errorf("no embedding data returned")
	}
	return response[0].Embedding[0], nil
}
