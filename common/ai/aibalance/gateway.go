package aibalance

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/twofa"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// TOTP 密钥在数据库中的存储键
	AIBALANCE_TOTP_SECRET_KEY = "AIBALANCE_CLIENT_TOTP_SECRET"
)

// TOTP 密钥内存缓存
var (
	totpSecretCache     string
	totpSecretCacheLock sync.RWMutex
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
	ch, err := aispec.StructuredStreamBase(
		g.targetUrl,
		g.config.Model,
		s,
		g.BuildHTTPOptions,
		g.config.StreamHandler,
		g.config.ReasonStreamHandler,
		g.config.HTTPErrorHandler,
	)
	if err != nil && g.isMemfitTOTPError(err) {
		// TOTP 认证失败，刷新密钥并重试
		log.Infof("TOTP authentication failed in StructuredStream, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()
		return aispec.StructuredStreamBase(
			g.targetUrl,
			g.config.Model,
			s,
			g.BuildHTTPOptions,
			g.config.StreamHandler,
			g.config.ReasonStreamHandler,
			g.config.HTTPErrorHandler,
		)
	}
	return ch, err
}

var _ aispec.AIClient = (*GatewayClient)(nil)

func (g *GatewayClient) Chat(s string, function ...any) (string, error) {
	result, err := aispec.ChatBase(
		g.targetUrl,
		g.config.Model,
		s,
		aispec.WithChatBase_Function(function),
		aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
		aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
		aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
		aispec.WithChatBase_ErrHandler(g.config.HTTPErrorHandler),
		aispec.WithChatBase_ImageRawInstance(g.config.Images...),
	)

	// 检查是否是 TOTP 认证失败
	if err != nil && g.isMemfitTOTPError(err) {
		// TOTP 认证失败，刷新密钥并重试
		log.Infof("TOTP authentication failed, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()

		// 重试请求
		return aispec.ChatBase(
			g.targetUrl,
			g.config.Model,
			s,
			aispec.WithChatBase_Function(function),
			aispec.WithChatBase_PoCOptions(g.BuildHTTPOptions),
			aispec.WithChatBase_StreamHandler(g.config.StreamHandler),
			aispec.WithChatBase_ReasonStreamHandler(g.config.ReasonStreamHandler),
			aispec.WithChatBase_ErrHandler(g.config.HTTPErrorHandler),
			aispec.WithChatBase_ImageRawInstance(g.config.Images...),
		)
	}

	return result, err
}

func (g *GatewayClient) ExtractData(msg string, desc string, fields map[string]any) (map[string]any, error) {
	result, err := aispec.ChatBasedExtractData(
		g.targetUrl,
		g.config.Model, msg, fields,
		g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler,
		g.config.Images...,
	)

	// 检查是否是 TOTP 认证失败
	if err != nil && g.isMemfitTOTPError(err) {
		log.Infof("TOTP authentication failed in ExtractData, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()

		return aispec.ChatBasedExtractData(
			g.targetUrl,
			g.config.Model, msg, fields,
			g.BuildHTTPOptions, g.config.StreamHandler, g.config.ReasonStreamHandler, g.config.HTTPErrorHandler,
			g.config.Images...,
		)
	}

	return result, err
}

func (g *GatewayClient) ChatStream(s string) (io.Reader, error) {
	reader, err := aispec.ChatWithStream(
		g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.config.ReasonStreamHandler,
		g.BuildHTTPOptions,
	)

	// 检查是否是 TOTP 认证失败
	if err != nil && g.isMemfitTOTPError(err) {
		log.Infof("TOTP authentication failed in ChatStream, refreshing secret and retrying...")
		g.refreshTOTPSecretAndSave()

		return aispec.ChatWithStream(
			g.targetUrl, g.config.Model, s, g.config.HTTPErrorHandler, g.config.ReasonStreamHandler,
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

// getTOTPSecret gets TOTP secret with priority:
// 1. Memory cache
// 2. Database
// 3. Fetch from server (and save to database)
func (g *GatewayClient) getTOTPSecret() string {
	// 1. Check memory cache first
	totpSecretCacheLock.RLock()
	if totpSecretCache != "" {
		defer totpSecretCacheLock.RUnlock()
		return totpSecretCache
	}
	totpSecretCacheLock.RUnlock()

	// 2. Try to load from database
	db := consts.GetGormProfileDatabase()
	if db != nil {
		secret := yakit.GetKey(db, AIBALANCE_TOTP_SECRET_KEY)
		if secret != "" {
			log.Infof("Loaded TOTP secret from database")
			totpSecretCacheLock.Lock()
			totpSecretCache = secret
			totpSecretCacheLock.Unlock()
			return secret
		}
	}

	// 3. Fetch from server and save to database
	secret := g.fetchTOTPSecretFromServer()
	if secret != "" {
		g.saveTOTPSecretToDatabase(secret)
		totpSecretCacheLock.Lock()
		totpSecretCache = secret
		totpSecretCacheLock.Unlock()
	}
	return secret
}

// saveTOTPSecretToDatabase saves the TOTP secret to database
func (g *GatewayClient) saveTOTPSecretToDatabase(secret string) {
	db := consts.GetGormProfileDatabase()
	if db != nil {
		err := yakit.SetKey(db, AIBALANCE_TOTP_SECRET_KEY, secret)
		if err != nil {
			log.Errorf("Failed to save TOTP secret to database: %v", err)
		} else {
			log.Infof("TOTP secret saved to database")
		}
	}
}

// fetchTOTPSecretFromServer fetches the TOTP UUID from the server
func (g *GatewayClient) fetchTOTPSecretFromServer() string {
	// Build the URL for fetching TOTP UUID
	baseURL := g.targetUrl
	// Replace /v1/chat/completions with /v1/memfit-totp-uuid
	totpURL := strings.Replace(baseURL, "/v1/chat/completions", "/v1/memfit-totp-uuid", 1)

	log.Infof("Fetching TOTP UUID from: %s", totpURL)

	// Make HTTP request
	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Accept": "application/json",
		}),
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

	log.Infof("Successfully fetched TOTP secret from server")
	return secret
}

// refreshTOTPSecretAndSave clears the cache, fetches new secret from server, and saves to database
func (g *GatewayClient) refreshTOTPSecretAndSave() {
	log.Infof("Refreshing TOTP secret due to authentication failure...")

	// Clear memory cache
	totpSecretCacheLock.Lock()
	totpSecretCache = ""
	totpSecretCacheLock.Unlock()

	// Fetch new secret from server
	secret := g.fetchTOTPSecretFromServer()
	if secret != "" {
		// Save to database
		g.saveTOTPSecretToDatabase(secret)

		// Update memory cache
		totpSecretCacheLock.Lock()
		totpSecretCache = secret
		totpSecretCacheLock.Unlock()

		log.Infof("TOTP secret refreshed and saved successfully")
	} else {
		log.Errorf("Failed to refresh TOTP secret from server")
	}
}

// generateTOTPCode generates TOTP code using the secret
func (g *GatewayClient) generateTOTPCode() string {
	secret := g.getTOTPSecret()
	if secret == "" {
		log.Errorf("Cannot generate TOTP code: no secret available")
		return ""
	}
	return twofa.GetUTCCode(secret)
}

func (g *GatewayClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	headers := map[string]string{
		"Content-Type":  "application/json; charset=UTF-8",
		"Accept":        "application/json",
		"Authorization": "Bearer " + g.config.APIKey,
	}

	// Add TOTP header for memfit models
	if g.isMemfitModel() {
		totpCode := g.generateTOTPCode()
		if totpCode != "" {
			// Base64 encode the TOTP code
			encodedCode := base64.StdEncoding.EncodeToString([]byte(totpCode))
			headers["X-Memfit-OTP-Auth"] = encodedCode
			log.Infof("Added TOTP auth header for memfit model: %s", g.config.Model)
		} else {
			log.Warnf("Failed to generate TOTP code for memfit model: %s", g.config.Model)
		}
	}

	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(headers),
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
	opts = append(opts, poc.WithTimeout(600))
	if g.config.Host != "" {
		opts = append(opts, poc.WithHost(g.config.Host))
	}
	if g.config.Port > 0 {
		opts = append(opts, poc.WithPort(g.config.Port))
	}
	return opts, nil
}
