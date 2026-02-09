package aibalance

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// ErrMemfitTOTPAuthFailed is the error type for TOTP authentication failures
var ErrMemfitTOTPAuthFailed = errors.New("memfit_totp_auth_failed")

type GatewayClient struct {
	config *aispec.AIConfig

	targetUrl string
}

func (g *GatewayClient) GetConfig() *aispec.AIConfig {
	return g.config
}

func (g *GatewayClient) SupportedStructuredStream() bool {
	return true
}

func (g *GatewayClient) GetModelList() ([]*aispec.ModelMeta, error) {
	return aispec.ListChatModels(g.targetUrl, g.BuildHTTPOptions)
}

func (g *GatewayClient) StructuredStream(s string, function ...any) (chan *aispec.StructuredData, error) {
	// 用于捕获 TOTP 错误的标志
	var totpErrorDetected bool

	// 包装的错误处理器
	wrappedErrorHandler := func(err error) {
		if err != nil && g.isMemfitModel() && g.isMemfitTOTPError(err) {
			totpErrorDetected = true
		}
		if g.config.HTTPErrorHandler != nil {
			g.config.HTTPErrorHandler(err)
		}
	}

	ch, err := aispec.StructuredStreamBase(
		g.targetUrl,
		g.config.Model,
		s,
		g.BuildHTTPOptions,
		g.config.StreamHandler,
		g.config.ReasonStreamHandler,
		wrappedErrorHandler,
	)

	// 检查是否是 TOTP 认证失败
	shouldRetry := (err != nil && g.isMemfitModel() && g.isMemfitTOTPError(err)) || totpErrorDetected

	if shouldRetry {
		log.Debugf("TOTP authentication issue in StructuredStream, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()
		totpErrorDetected = false

		return aispec.StructuredStreamBase(
			g.targetUrl,
			g.config.Model,
			s,
			g.BuildHTTPOptions,
			g.config.StreamHandler,
			g.config.ReasonStreamHandler,
			wrappedErrorHandler,
		)
	}
	return ch, err
}

var _ aispec.AIClient = (*GatewayClient)(nil)

func (g *GatewayClient) Chat(s string, function ...any) (string, error) {
	// 用于捕获 TOTP 错误的标志
	var totpErrorDetected bool

	// 包装的错误处理器，用于检测 TOTP 错误
	wrappedErrorHandler := func(err error) {
		if err != nil && g.isMemfitModel() && g.isMemfitTOTPError(err) {
			totpErrorDetected = true
		}
		if g.config.HTTPErrorHandler != nil {
			g.config.HTTPErrorHandler(err)
		}
	}

	// 检测 TOTP 错误并刷新
	shouldRefreshAndRetry := func(result string, err error) bool {
		// 只有 memfit 模型才需要检测 TOTP 错误
		if !g.isMemfitModel() {
			return false
		}

		// 检查标志（由错误处理器设置）
		if totpErrorDetected {
			return true
		}

		// 检查错误中是否包含 TOTP 错误
		if err != nil && g.isMemfitTOTPError(err) {
			return true
		}

		// 检查结果中是否包含 TOTP 错误（流式请求可能没有正确返回错误）
		if isMemfitTOTPErrorInResponse(result) {
			return true
		}

		// memfit 模型返回空结果时，尝试刷新 TOTP（可能是认证失败导致）
		// 注意：这是一个保守策略，只对 memfit 模型生效
		if result == "" && err == nil {
			log.Debugf("Empty result for memfit model, may be TOTP auth issue, will try refresh")
			return true
		}

		return false
	}

	result, err := aispec.ChatBase(
		g.targetUrl,
		g.config.Model,
		s,
		aispec.WithChatBase_Function(function),
		aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(wrappedErrorHandler),
		aispec.WithChatBase_ImageRawInstance(g.config.Images...),
		aispec.WithChatBase_Tools(g.config.Tools),
		aispec.WithChatBase_ToolChoice(g.config.ToolChoice),
		aispec.WithChatBase_ToolCallCallback(g.config.ToolCallCallback),
	)

	// 检查是否是 TOTP 认证失败（需要刷新密钥并重试）
	if shouldRefreshAndRetry(result, err) {
		log.Debugf("TOTP authentication issue for memfit model, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()

		// 重置标志
		totpErrorDetected = false

		// 重试请求
		return aispec.ChatBase(
			g.targetUrl,
			g.config.Model,
			s,
			aispec.WithChatBase_Function(function),
			aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
			aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
			aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
			aispec.WithChatBase_ErrHandler(wrappedErrorHandler),
			aispec.WithChatBase_ImageRawInstance(g.config.Images...),
			aispec.WithChatBase_Tools(g.config.Tools),
			aispec.WithChatBase_ToolChoice(g.config.ToolChoice),
			aispec.WithChatBase_ToolCallCallback(g.config.ToolCallCallback),
		)
	}

	return result, err
}

func (g *GatewayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	// 用于捕获 TOTP 错误的标志
	var totpErrorDetected bool

	// 包装的错误处理器
	wrappedErrorHandler := func(err error) {
		if err != nil && g.isMemfitModel() && g.isMemfitTOTPError(err) {
			totpErrorDetected = true
		}
		if g.config.HTTPErrorHandler != nil {
			g.config.HTTPErrorHandler(err)
		}
	}

	// 检测 TOTP 错误并刷新
	shouldRefreshAndRetry := func(result map[string]any, err error) bool {
		// 只有 memfit 模型才需要检测 TOTP 错误
		if !g.isMemfitModel() {
			return false
		}

		// 检查标志
		if totpErrorDetected {
			return true
		}

		// 检查错误中是否包含 TOTP 错误
		if err != nil && g.isMemfitTOTPError(err) {
			return true
		}

		return false
	}

	result, err := aispec.ChatBasedExtractData(
		g.targetUrl,
		g.config.Model, msg, fields,
		g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, wrappedErrorHandler,
		g.config.Images...,
	)

	// 检查是否是 TOTP 认证失败
	if shouldRefreshAndRetry(result, err) {
		log.Debugf("TOTP authentication issue in ExtractData, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()

		// 重置标志
		totpErrorDetected = false

		return aispec.ChatBasedExtractData(
			g.targetUrl,
			g.config.Model, msg, fields,
			g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, wrappedErrorHandler,
			g.config.Images...,
		)
	}

	return result, err
}

func (g *GatewayClient) ChatStream(s string) (io.Reader, error) {
	// 用于捕获 TOTP 错误的标志
	var totpErrorDetected bool

	// 包装的错误处理器
	wrappedErrorHandler := func(err error) {
		if err != nil && g.isMemfitModel() && g.isMemfitTOTPError(err) {
			totpErrorDetected = true
		}
		if g.config.HTTPErrorHandler != nil {
			g.config.HTTPErrorHandler(err)
		}
	}

	reader, err := aispec.ChatWithStream(
		g.targetUrl, g.config.Model, s, wrappedErrorHandler, g.config.ReasonStreamHandler,
		g.BuildHTTPOptions,
	)

	// 检查是否是 TOTP 认证失败
	shouldRetry := (err != nil && g.isMemfitModel() && g.isMemfitTOTPError(err)) || totpErrorDetected

	if shouldRetry {
		log.Debugf("TOTP authentication issue in ChatStream, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()
		totpErrorDetected = false

		return aispec.ChatWithStream(
			g.targetUrl, g.config.Model, s, wrappedErrorHandler, g.config.ReasonStreamHandler,
			g.BuildHTTPOptions,
		)
	}

	return reader, err
}

// isMemfitTOTPError checks if the error is a TOTP authentication failure
func (g *GatewayClient) isMemfitTOTPError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// 检查错误消息中是否包含 TOTP 认证失败的标识
	return strings.Contains(errStr, "memfit_totp_auth_failed") ||
		strings.Contains(errStr, "Memfit TOTP authentication failed") ||
		strings.Contains(errStr, "memfit_totp_auth_required")
}

// isMemfitTOTPErrorInResponse checks if the response content contains TOTP error
func isMemfitTOTPErrorInResponse(content string) bool {
	return strings.Contains(content, "memfit_totp_auth_failed") ||
		strings.Contains(content, "Memfit TOTP authentication failed")
}

func (g *GatewayClient) newLoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "deepseek-v3"
	}

	g.targetUrl = aispec.GetBaseURLFromConfig(g.config, "https://aibalance.yaklang.com", "/v1/chat/completions")
}

func (g *GatewayClient) LoadOption(opt ...aispec.AIConfigOption) {
	if aispec.EnableNewLoadOption {
		g.newLoadOption(opt...)
		return
	}
	config := aispec.NewDefaultAIConfig(opt...)
	g.config = config

	if g.config.Model == "" {
		g.config.Model = "deepseek-v3"
	}

	if config.BaseURL != "" {
		g.targetUrl = config.BaseURL
	} else if config.Domain != "" {
		if config.NoHttps {
			g.targetUrl = "http://" + config.Domain + "/v1/chat/completions"
		} else {
			g.targetUrl = "https://" + config.Domain + "/v1/chat/completions"
		}
	} else {
		g.targetUrl = "https://aibalance.yaklang.com/v1/chat/completions"
	}
}

func (g *GatewayClient) CheckValid() error {
	if g.config.APIKey == "" {
		return errors.New("APIKey is required")
	}
	return nil
}

// isMemfitModel checks if the model name starts with "memfit-"
func (g *GatewayClient) isMemfitModel() bool {
	return strings.HasPrefix(strings.ToLower(g.config.Model), "memfit-")
}

// getTOTPSecret gets TOTP secret using the shared cache (aibalanceclient package)
// Priority: Memory cache -> Database -> Fetch from server
// The cache is shared with omnisearch AiBalanceSearchClient
func (g *GatewayClient) getTOTPSecret() string {
	return aibalanceclient.GetOrFetchTOTPSecret(g.fetchTOTPSecretFromServer)
}

// fetchTOTPSecretFromServer fetches the TOTP UUID from the server
func (g *GatewayClient) fetchTOTPSecretFromServer() string {
	// Build the URL for fetching TOTP UUID
	baseURL := g.targetUrl
	// Replace /v1/chat/completions with /v1/memfit-totp-uuid
	totpURL := strings.Replace(baseURL, "/v1/chat/completions", "/v1/memfit-totp-uuid", 1)

	log.Debugf("Fetching TOTP UUID from: %s", totpURL)

	// Make HTTP request with connection pool enabled
	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Accept":          "application/json",
			"Accept-Encoding": "gzip, deflate, br", // enable compression for better network performance
		}),
		poc.WithConnPool(true),     // enable connection pool for better performance
		poc.WithSave(false),        // do not save TOTP requests to database
		poc.WithConnectTimeout(10), // set connect timeout
		poc.WithRetryTimes(2),      // retry on failure
	}
	if g.config.Proxy != "" {
		opts = append(opts, poc.WithProxy(g.config.Proxy))
	}
	if g.config.Timeout > 0 {
		opts = append(opts, poc.WithTimeout(g.config.Timeout))
	}

	rsp, _, err := poc.DoGET(totpURL, opts...)
	if err != nil {
		log.Errorf("Failed to fetch TOTP UUID: %v", err)
		return ""
	}

	// Parse response
	var result struct {
		UUID   string `json:"uuid"`
		Format string `json:"format"`
	}

	body := rsp.GetBody()
	if err := json.Unmarshal(body, &result); err != nil {
		log.Errorf("Failed to parse TOTP UUID response: %v", err)
		return ""
	}

	// Extract secret from wrapped UUID: MEMFIT-AI<uuid>MEMFIT-AI
	uuid := result.UUID
	if uuid == "" {
		log.Errorf("Empty TOTP UUID in response")
		return ""
	}

	// Remove MEMFIT-AI prefix and suffix
	secret := strings.TrimPrefix(uuid, "MEMFIT-AI")
	secret = strings.TrimSuffix(secret, "MEMFIT-AI")

	log.Debugf("Successfully fetched TOTP secret from server")
	return secret
}

// refreshTOTPSecretAndSave clears the shared cache, fetches new secret from server, and saves
// This function is called when TOTP authentication fails
func (g *GatewayClient) refreshTOTPSecretAndSave() {
	log.Debugf("Refreshing TOTP secret due to authentication failure...")
	aibalanceclient.RefreshTOTPSecret(g.fetchTOTPSecretFromServer)
}

// generateTOTPCode generates TOTP code using the shared cached secret
func (g *GatewayClient) generateTOTPCode() string {
	return aibalanceclient.GenerateTOTPCode(g.fetchTOTPSecretFromServer)
}

func (g *GatewayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	headers := map[string]string{
		"Content-Type":    "application/json; charset=UTF-8",
		"Accept":          "application/json",
		"Accept-Encoding": "gzip, deflate, br", // enable compression for better network performance
		"Authorization":   "Bearer " + g.config.APIKey,
	}

	// Add TOTP header for memfit models
	if g.isMemfitModel() {
		totpCode := g.generateTOTPCode()
		if totpCode != "" {
			// Base64 encode the TOTP code
			encodedCode := base64.StdEncoding.EncodeToString([]byte(totpCode))
			headers["X-Memfit-OTP-Auth"] = encodedCode
			// Note: Removed verbose log to keep logs clean during normal operation
			// TOTP header is silently added for memfit models
		} else {
			log.Warnf("Failed to generate TOTP code for memfit model: %s", g.config.Model)
		}
	}

	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(headers),
		poc.WithConnPool(true), // enable connection pool for better performance
		poc.WithSave(false),    // do not save AI chat requests to database
	}
	opts = append(opts, poc.WithTimeout(g.config.Timeout))
	if g.config.Proxy != "" {
		opts = append(opts, poc.WithProxy(g.config.Proxy))
	}
	if g.config.Context != nil {
		opts = append(opts, poc.WithContext(g.config.Context))
	}
	if g.config.Timeout > 0 {
		opts = append(opts, poc.WithConnectTimeout(g.config.Timeout))
	}
	// Use a reasonable timeout: 200 seconds for AI requests
	// This prevents goroutine leaks when AI providers hang
	opts = append(opts, poc.WithTimeout(200))
	if g.config.Host != "" {
		opts = append(opts, poc.WithHost(g.config.Host))
	}
	if g.config.Port > 0 {
		opts = append(opts, poc.WithPort(g.config.Port))
	}
	return opts, nil
}
