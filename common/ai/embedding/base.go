package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
)

type OpenaiEmbeddingClient struct {
	config     *aispec.AIConfig
	httpClient *http.Client
}

var _ aispec.EmbeddingCaller = (*OpenaiEmbeddingClient)(nil)

func NewOpenaiEmbeddingClient(opt ...aispec.AIConfigOption) *OpenaiEmbeddingClient {
	config := aispec.NewDefaultAIConfig(opt...)
	c := &OpenaiEmbeddingClient{
		config:     config,
		httpClient: newEmbeddingHTTPClient(config),
	}
	return c
}

const (
	embeddingConnectTimeout   = 5 * time.Second
	embeddingTransportRetries = 5
	embeddingErrorPreviewSize = 500
)

func newEmbeddingHTTPClient(config *aispec.AIConfig) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ForceAttemptHTTP2 = true

	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		connectCtx, cancel := context.WithTimeout(ctx, embeddingConnectTimeout)
		defer cancel()
		proxies := cli.DefaultCliApp.PeekStringSlice("proxy")
		if proxy := strings.TrimSpace(config.Proxy); proxy != "" {
			proxies = []string{proxy}
		}
		if len(proxies) == 0 {
			return netx.DialContext(connectCtx, address)
		}
		return netx.DialContext(connectCtx, address, proxies...)
	}

	timeout := utils.FloatSecondDuration(config.Timeout)
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

type embeddingRequest struct {
	Input          string `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Model          string `json:"model,omitempty"`
}

type embeddingItem struct {
	Embedding embeddingValue `json:"embedding"`
}

// embeddingValue decodes the vector while the enclosing item is being
// decoded. This avoids retaining a copy of the complete vector as a
// json.RawMessage and unmarshalling that copy in a second step.
type embeddingValue struct {
	vector  []float32
	vectors [][]float32
	multi   bool
}

func (value *embeddingValue) UnmarshalJSON(raw []byte) error {
	*value = embeddingValue{}
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' {
		return nil
	}
	inner := bytes.TrimSpace(raw[1:])
	if len(inner) > 0 && inner[0] == '[' {
		value.multi = true
		value.vectors = make([][]float32, 0, countTopLevelJSONElements(raw))
		return json.Unmarshal(raw, &value.vectors)
	}
	value.vector = make([]float32, 0, countFlatJSONElements(raw))
	return json.Unmarshal(raw, &value.vector)
}

// countFlatJSONElements counts numeric array elements without fully parsing
// them. Strings and nested values are rejected by []float32 unmarshalling, so
// they deliberately disable preallocation rather than using their commas.
func countFlatJSONElements(raw []byte) int {
	if len(raw) < 2 || raw[0] != '[' {
		return 0
	}
	index := 1
	for index < len(raw) && isJSONSpace(raw[index]) {
		index++
	}
	if index >= len(raw) || raw[index] == ']' {
		return 0
	}

	count := 1
	for ; index < len(raw); index++ {
		switch raw[index] {
		case ',':
			count++
		case '"', '[', '{':
			return 0
		case ']':
			// A valid JSON array needs at least one byte per value and one
			// separator between values. Avoid amplifying malformed input.
			if count > (len(raw)-1)/2 {
				return 0
			}
			return count
		}
	}
	return 0
}

// countTopLevelJSONElements returns the number of top-level array elements without
// interpreting commas inside strings or nested JSON values. Supplying the
// capacity to encoding/json avoids the repeated slice growth that otherwise
// dominates allocations for large embedding vectors.
func countTopLevelJSONElements(raw []byte) int {
	if len(raw) < 2 || raw[0] != '[' {
		return 0
	}
	index := 1
	for index < len(raw) && isJSONSpace(raw[index]) {
		index++
	}
	if index >= len(raw) || raw[index] == ']' {
		return 0
	}

	count, depth := 1, 1
	inString, escaped := false, false
	for ; index < len(raw); index++ {
		character := raw[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return count
			}
		case ',':
			if depth == 1 {
				count++
			}
		}
	}
	return count
}

func isJSONSpace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\r' || character == '\n'
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

	rsp, err := c.doEmbeddingRequest(targetUrl, jsonData)
	if err != nil {
		log.Errorf("Embedding request failed: %v", err)
		return nil, utils.Errorf("request embeddings failed: %v", err)
	}
	defer rsp.Body.Close()

	preview := &responsePreviewReader{reader: rsp.Body}
	vectors, errResp, decodeErr := decodeEmbeddingResponse(preview)
	if decodeErr == nil {
		// Ensure the transport can reuse the connection even if the decoder read
		// exactly through the closing JSON delimiter without observing EOF.
		_, _ = io.Copy(io.Discard, rsp.Body)
	}
	if rsp.StatusCode >= http.StatusBadRequest {
		log.Warnf("Embedding response error body: %s", preview.String())
	} else if rsp.StatusCode != http.StatusOK {
		log.Infof("Embedding response status: %d, content length: %d", rsp.StatusCode, rsp.ContentLength)
	}
	if len(vectors) > 0 {
		normalizeEmbeddingVectors(vectors)
		return vectors, nil
	}
	if errResp != nil && errResp.Error.Message != "" {
		// 检查是否包含 "input is too large" 错误
		if strings.Contains(strings.ToLower(errResp.Error.Message), "input is too large") {
			return nil, ErrInputTooLarge
		}
		// 返回其他API错误
		return nil, utils.Errorf("API error: %s (code: %d, type: %s)",
			errResp.Error.Message, errResp.Error.Code, errResp.Error.Type)
	}
	if decodeErr != nil {
		return nil, utils.Errorf("failed to parse embedding response: %v; body: %s", decodeErr, preview.String())
	}
	return nil, utils.Errorf("failed to parse embedding response in any known format: %s", preview.String())
}

func (c *OpenaiEmbeddingClient) doEmbeddingRequest(targetURL string, body []byte) (*http.Response, error) {
	ctx := c.config.Context
	if ctx == nil {
		ctx = context.Background()
	}

	var lastErr error
	for attempt := 0; attempt <= embeddingTransportRetries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json; charset=UTF-8")
		request.Header.Set("Accept", "application/json")
		if c.config.APIKey != "" {
			request.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}
		for _, header := range c.config.Headers {
			if header == nil || strings.TrimSpace(header.GetKey()) == "" {
				continue
			}
			request.Header.Set(header.GetKey(), header.GetValue())
		}

		response, err := c.httpClient.Do(request)
		if err == nil {
			return response, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == embeddingTransportRetries {
			break
		}
		if err := waitForEmbeddingRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func waitForEmbeddingRetry(ctx context.Context, attempt int) error {
	if attempt > 4 {
		attempt = 4
	}
	delay := 100 * time.Millisecond << attempt
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type responsePreviewReader struct {
	reader    io.Reader
	preview   []byte
	truncated bool
}

func (r *responsePreviewReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	remaining := embeddingErrorPreviewSize - len(r.preview)
	if remaining > 0 && n > 0 {
		copyLength := n
		if copyLength > remaining {
			copyLength = remaining
			r.truncated = true
		}
		r.preview = append(r.preview, buffer[:copyLength]...)
	} else if n > 0 {
		r.truncated = true
	}
	return n, err
}

func (r *responsePreviewReader) String() string {
	preview := strings.TrimSpace(string(r.preview))
	if r.truncated {
		preview += "... (truncated)"
	}
	return preview
}

func decodeEmbeddingResponse(reader io.Reader) ([][]float32, *errorResponse, error) {
	decoder := json.NewDecoder(reader)
	first, err := decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	delim, ok := first.(json.Delim)
	if !ok {
		return nil, nil, utils.Errorf("unexpected top-level JSON token %v", first)
	}

	switch delim {
	case '[':
		vectors, err := decodeEmbeddingItems(decoder, true, false)
		return vectors, nil, err
	case '{':
		var vectors [][]float32
		var apiError errorResponse
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, nil, utils.Errorf("unexpected object key %v", keyToken)
			}
			switch key {
			case "data":
				vectors, err = decodeEmbeddingItems(decoder, false, true)
			case "error":
				err = decoder.Decode(&apiError.Error)
			default:
				err = skipJSONValue(decoder)
			}
			if err != nil {
				return nil, nil, err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return nil, nil, err
		}
		if apiError.Error.Message != "" {
			return vectors, &apiError, nil
		}
		return vectors, nil, nil
	default:
		return nil, nil, utils.Errorf("unexpected top-level JSON delimiter %q", delim)
	}
}

// decodeEmbeddingItems parses either the object envelope's data array or a
// provider-specific top-level item array. Object responses and 2-D top-level
// responses intentionally preserve the legacy behavior of using the first
// item; top-level 1-D item arrays return one vector per item.
func decodeEmbeddingItems(decoder *json.Decoder, arrayAlreadyOpen, firstItemOnly bool) ([][]float32, error) {
	if !arrayAlreadyOpen {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if token == nil {
			return nil, nil
		}
		if token != json.Delim('[') {
			return nil, utils.Errorf("embedding data is not an array")
		}
	}

	var vectors [][]float32
	isMultiVector := false
	itemIndex := 0
	for decoder.More() {
		if itemIndex > 0 && (firstItemOnly || isMultiVector || len(vectors) == 0) {
			if err := skipJSONValue(decoder); err != nil {
				return nil, err
			}
			itemIndex++
			continue
		}
		var item embeddingItem
		if err := decoder.Decode(&item); err != nil {
			return nil, err
		}
		if itemIndex == 0 {
			isMultiVector = item.Embedding.multi
			if isMultiVector {
				vectors = append(vectors, item.Embedding.vectors...)
			} else if len(item.Embedding.vector) > 0 {
				vectors = append(vectors, item.Embedding.vector)
			}
		} else if vector, ok := item.Embedding.firstVector(); ok {
			vectors = append(vectors, vector)
		}
		itemIndex++
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return vectors, nil
}

func (value *embeddingValue) firstVector() ([]float32, bool) {
	if value.multi {
		if len(value.vectors) == 0 {
			return nil, false
		}
		return value.vectors[0], true
	}
	if len(value.vector) == 0 {
		return nil, false
	}
	return value.vector, true
}

func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || (delim != '{' && delim != '[') {
		return nil
	}
	for decoder.More() {
		if delim == '{' {
			if _, err := decoder.Token(); err != nil {
				return err
			}
		}
		if err := skipJSONValue(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func normalizeEmbeddingVectors(vectors [][]float32) {
	for _, vector := range vectors {
		if len(vector) == 0 {
			continue
		}
		norm := computePNorm1D(vector, 2)
		if norm < 1e-6 {
			norm = 1e-6
		}
		inverse := 1.0 / norm
		for index := range vector {
			vector[index] = float32(float64(vector[index]) * inverse)
		}
	}
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
