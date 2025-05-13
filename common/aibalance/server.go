package aibalance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/aibalance/aiforwarder"
	"github.com/yaklang/yaklang/common/utils/omap"

	_ "github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// LogLevel represents the logging level configuration
type LogLevel struct {
	Debug bool `json:"debug"` // Enable debug level logging
	Info  bool `json:"info"`  // Enable info level logging
	Warn  bool `json:"warn"`  // Enable warning level logging
	Error bool `json:"error"` // Enable error level logging
}

// Key represents an API key with its permissions
type Key struct {
	Key           string
	AllowedModels map[string]bool
}

// KeyManager manages API keys and their permissions
type KeyManager struct {
	keys map[string]*Key
}

// NewKeyManager creates a new key manager
func NewKeyManager() *KeyManager {
	return &KeyManager{
		keys: make(map[string]*Key),
	}
}

// Get retrieves a key from the manager
func (k *KeyManager) Get(key string) (*Key, bool) {
	v, ok := k.keys[key]
	return v, ok
}

// KeyAllowedModels manages allowed models for each key
type KeyAllowedModels struct {
	allowedModels map[string]map[string]bool
}

// NewKeyAllowedModels creates a new key allowed models manager
func NewKeyAllowedModels() *KeyAllowedModels {
	return &KeyAllowedModels{
		allowedModels: make(map[string]map[string]bool),
	}
}

// Get retrieves allowed models for a key
func (k *KeyAllowedModels) Get(key string) (map[string]bool, bool) {
	v, ok := k.allowedModels[key]
	return v, ok
}

// Keys returns all keys
func (k *KeyAllowedModels) Keys() []string {
	keys := make([]string, 0, len(k.allowedModels))
	for k := range k.allowedModels {
		keys = append(keys, k)
	}
	return keys
}

// ModelManager manages AI models and their providers
type ModelManager struct {
	models map[string][]*Provider
}

// NewModelManager creates a new model manager
func NewModelManager() *ModelManager {
	return &ModelManager{
		models: make(map[string][]*Provider),
	}
}

// Get retrieves a model from the manager
func (m *ModelManager) Get(name string) ([]*Provider, bool) {
	v, ok := m.models[name]
	return v, ok
}

// Entrypoints manages model providers
type Entrypoints struct {
	providers map[string][]*Provider
}

// NewEntrypoints creates a new entrypoints manager
func NewEntrypoints() *Entrypoints {
	return &Entrypoints{
		providers: make(map[string][]*Provider),
	}
}

// PeekProvider returns a random provider for the given model
func (e *Entrypoints) PeekProvider(model string) *Provider {
	providers, ok := e.providers[model]
	if !ok || len(providers) == 0 {
		return nil
	}
	// Return the first provider
	return providers[0]
}

// GetAllProviders returns all providers for the given model
func (e *Entrypoints) GetAllProviders(model string) []*Provider {
	return e.providers[model]
}

// Add adds providers to the model
func (e *Entrypoints) Add(model string, providers []*Provider) {
	if _, ok := e.providers[model]; !ok {
		e.providers[model] = make([]*Provider, 0)
	}
	e.providers[model] = append(e.providers[model], providers...)
}

// LoadAPIKeysFromDB 从数据库加载API密钥到内存配置
func (c *ServerConfig) LoadAPIKeysFromDB() error {
	log.Info("Loading API keys from database...")

	// 从数据库获取所有API密钥
	apiKeys, err := GetAllAiApiKeys()
	if err != nil {
		return fmt.Errorf("failed to load API keys from database: %v", err)
	}

	// 获取所有提供者的 WrapperName
	providers, err := GetAllAiProviders()
	if err != nil {
		return fmt.Errorf("failed to load providers from database: %v", err)
	}

	// 创建 WrapperName 映射
	wrapperNames := make(map[string]bool)
	for _, p := range providers {
		if p.WrapperName != "" {
			wrapperNames[p.WrapperName] = true
		}
	}

	// 清空当前内存中的配置
	c.KeyAllowedModels.allowedModels = make(map[string]map[string]bool)
	c.Keys.keys = make(map[string]*Key) // 同时清空 Keys 结构

	// 加载到内存配置
	for _, key := range apiKeys {
		// 解析允许的模型列表
		modelNames := strings.Split(key.AllowedModels, ",")
		modelMap := make(map[string]bool)
		for _, model := range modelNames {
			model = strings.TrimSpace(model)
			if model != "" && wrapperNames[model] {
				modelMap[model] = true
			}
		}

		// 添加到 KeyAllowedModels
		c.KeyAllowedModels.allowedModels[key.APIKey] = modelMap

		// 同时添加到 Keys 结构
		c.Keys.keys[key.APIKey] = &Key{
			Key:           key.APIKey,
			AllowedModels: modelMap,
		}

		log.Infof("Loaded API key: %s with allowed models: %v", utils.ShrinkString(key.APIKey, 8), modelMap)
	}

	log.Infof("Successfully loaded %d API keys from database", len(apiKeys))
	return nil
}

// ServerConfig represents the server configuration
type ServerConfig struct {
	Keys             *KeyManager
	KeyAllowedModels *KeyAllowedModels
	Models           *ModelManager
	Entrypoints      *Entrypoints
	Logging          LogLevel
	AdminPassword    string          // 添加管理员密码配置
	SessionManager   *SessionManager // 会话管理器
	forwardRule      *omap.OrderedMap[string, *aiforwarder.Rule]
}

// NewServerConfig creates a new server configuration
func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		Keys:             NewKeyManager(),
		KeyAllowedModels: NewKeyAllowedModels(),
		Models:           NewModelManager(),
		Entrypoints:      NewEntrypoints(),
		Logging: LogLevel{
			Debug: true,
			Info:  true,
			Warn:  true,
			Error: true,
		},
		AdminPassword:  "admin", // 默认密码
		SessionManager: NewSessionManager(),
		forwardRule:    omap.NewOrderedMap[string, *aiforwarder.Rule](make(map[string]*aiforwarder.Rule)),
	}
}

// logDebug logs a debug message if debug logging is enabled
func (c *ServerConfig) logDebug(format string, args ...interface{}) {
	if c.Logging.Debug {
		log.Debugf(format, args...)
	}
}

// logInfo logs an info message if info logging is enabled
func (c *ServerConfig) logInfo(format string, args ...interface{}) {
	if c.Logging.Info {
		log.Infof(format, args...)
	}
}

// logWarn logs a warning message if warning logging is enabled
func (c *ServerConfig) logWarn(format string, args ...interface{}) {
	if c.Logging.Warn {
		log.Warnf(format, args...)
	}
}

// logError logs an error message if error logging is enabled
func (c *ServerConfig) logError(format string, args ...interface{}) {
	if c.Logging.Error {
		log.Errorf(format, args...)
	}
}

func (c *ServerConfig) getKeyFromRawRequest(req []byte) *Key {
	header := lowhttp.GetHTTPPacketHeader(req, "Authorization")
	key := strings.TrimPrefix(header, "Bearer ")
	l, ok := c.Keys.Get(key)
	if ok {
		return l
	}
	return nil
}

func (c *ServerConfig) serveChatCompletions(conn net.Conn, rawPacket []byte) {
	c.logInfo("Starting to handle new chat completion request")
	// handle ai request
	auth := ""
	_, body := lowhttp.SplitHTTPPacket(rawPacket, func(method string, requestUri string, proto string) error {
		c.logInfo("Request method: %s, URI: %s, Protocol: %s", method, requestUri, proto)
		return nil
	}, func(proto string, code int, codeMsg string) error {
		return nil
	}, func(line string) string {
		k, v := lowhttp.SplitHTTPHeader(line)
		if k == "Authorization" || k == "authorization" {
			auth = v
			c.logInfo("Retrieved authentication info from request header: %s", v)
		}
		return line
	})
	if string(body) == "" {
		c.logError("Request body is empty")
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}
	value := strings.TrimPrefix(auth, "Bearer ")
	c.logInfo("Extracted key from authentication info: %s", value)
	if value == "" {
		c.logError("No valid authentication info provided")
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return
	}

	key, ok := c.Keys.Get(value)
	if !ok {
		c.logError("No matching key configuration found: %s", value)
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return
	}

	c.logInfo("Successfully verified key: %s", key.Key)
	_ = key
	_ = body

	var bodyIns aispec.ChatMessage
	err := json.Unmarshal([]byte(body), &bodyIns)
	if err != nil {
		c.logError("Failed to parse request body: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	stream := bodyIns.Stream
	log.Infof("user require stream flag: %v", stream)

	modelName := bodyIns.Model
	c.logInfo("Requested model: %s", modelName)

	var prompt bytes.Buffer
	for _, message := range bodyIns.Messages {
		prompt.WriteString(message.Content + "\n")
	}

	if prompt.Len() == 0 {
		c.logError("Prompt is empty")
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nX-Reason: empty prompt\r\n\r\n"))
		return
	}

	c.logInfo("Built prompt length: %d", prompt.Len())

	allowedModels, ok := c.KeyAllowedModels.Get(key.Key)
	if !ok {
		c.logError("Key[%v] has no allowed models configured", key.Key)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}

	isAllowed, ok := allowedModels[modelName]
	if !ok {
		allowedModelKeys := make([]string, 0, len(allowedModels))
		for k := range allowedModels {
			allowedModelKeys = append(allowedModelKeys, k)
		}
		c.logError("Key[%v] requested model %s is not in allowed list, allowed models: %v", key.Key, modelName, allowedModelKeys)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}

	if !isAllowed {
		c.logError("Key[%v] requested model %s is not allowed", key.Key, modelName)
		conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
		return
	}

	model, ok := c.Models.Get(modelName)
	if !ok {
		c.logError("No model configuration found: %s", modelName)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}

	c.logInfo("Key[%v] requesting model %s, starting to forward request", key.Key, modelName)
	_ = model
	provider := c.Entrypoints.PeekProvider(modelName)
	if provider == nil {
		c.logError("No provider found for model %s", modelName)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 404 Not Found\r\nX-Reason: no provider found, contact admin to add provider for %v\r\n\r\n", modelName)))
		return
	}

	c.logInfo("Found provider for model %s: %s", modelName, provider.TypeName)
	c.logInfo("Starting to call AI chat interface")

	sendHeaderOnce := sync.Once{}
	sendHeader := func() {
		c.logInfo("Successfully obtained AI client, starting to send response header")
		var header string
		header = "HTTP/1.1 200 OK\r\n" +
			"Content-Type: application/json\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"\r\n"
		_, err := conn.Write([]byte(header))
		if err != nil {
			c.logError("Failed to send response header: %v", err)
		}
		c.logInfo("Response header sent, bytes: %d", len(header))
		utils.FlushWriter(conn)
	}
	pr, pw := utils.NewBufPipe(nil)
	rr, rw := utils.NewBufPipe(nil)

	// baseCtx, cancel := context.WithCancel(context.Background())

	writer := NewChatJSONChunkWriter(conn, key.Key, modelName)
	client, err := provider.GetAIClient(func(reader io.Reader) {
		defer func() {
			pw.Close()
			c.logInfo("Finished handling AI response stream(output)")
		}()
		c.logInfo("Start to handle AI response stream")
		sendHeaderOnce.Do(sendHeader)
		io.Copy(pw, reader)
	}, func(reader io.Reader) {
		defer func() {
			rw.Close()
			c.logInfo("Finished handling AI response stream(reason)")
		}()
		c.logInfo("Start to handle AI response stream(reason)")
		sendHeaderOnce.Do(sendHeader)
		io.Copy(rw, reader)
		utils.FlushWriter(writer.writerClose)
	})
	if err != nil {
		c.logError("Failed to get AI client: %v", err)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nX-Reason: %v\r\n\r\n", err)))
		return
	}
	go func() {
		c.logInfo("start to call ai chat interface with prompt len: %d", prompt.Len())
		finalMsg, err := client.Chat(prompt.String())
		if err != nil {
			c.logError("AI chat interface call failed: %v", err)
			return
		}
		c.logInfo("AI chat interface call completed, final: %v", utils.ShrinkString(finalMsg, 100))
		_ = finalMsg
	}()

	wg := new(sync.WaitGroup)
	wg.Add(2)

	reasonWriter := writer.GetReasonWriter()
	outputWriter := writer.GetOutputWriter()

	// Handle reason stream
	start := time.Now()
	firstByteDuration := time.Duration(0)
	fonce := sync.Once{}
	totalBytes := new(int64)

	go func() {
		defer func() {
			c.logInfo("Finished forwarding AI response stream(reason)")
			wg.Done()
		}()
		c.logInfo("Start to handle reason mirror stream")
		n, err := io.Copy(reasonWriter, io.TeeReader(rr, utils.FirstWriter(func(p []byte) {
			fonce.Do(func() {
				firstByteDuration = time.Since(start)
			})
		})))
		if err != nil {
			c.logError("Failed to copy reason stream: %v", err)
		}
		atomic.AddInt64(totalBytes, n)
		c.logInfo("Reason stream copy completed, bytes: %d", n)
	}()

	// Handle output stream
	go func() {
		defer func() {
			c.logInfo("Finished forwarding AI response stream(output)")
			wg.Done()
		}()
		c.logInfo("Start to handle output mirror stream")
		n, err := io.Copy(outputWriter, io.TeeReader(pr, utils.FirstWriter(func(p []byte) {
			fonce.Do(func() {
				firstByteDuration = time.Since(start)
			})
		})))
		atomic.AddInt64(totalBytes, n)
		if err != nil {
			c.logError("Failed to copy output stream: %v", err)
		}
		c.logInfo("Output stream copy completed, bytes: %d", n)
	}()

	// Wait for all stream processing to complete
	wg.Wait()
	utils.FlushWriter(writer.writerClose)

	if !stream {
		body = writer.GetNotStreamBody()
		cwr := httputil.NewChunkedWriter(conn)
		cwr.Write(body)
		cwr.Close()
		utils.FlushWriter(cwr)
		utils.FlushWriter(conn)
	}

	endDuration := time.Since(start)
	total := atomic.LoadInt64(totalBytes)
	requestSucceeded := total > 0 // Determine actual request success based on data received
	if !requestSucceeded {
		// Log and write error only if no data was received
		c.logWarn("No data received from provider for model %s, key: %s", modelName, utils.ShrinkString(key.Key, 8))
		// writer.WriteError(fmt.Errorf("no data received from provider")) // Avoid writing error here if connection closed normally
	}

	// Update provider status
	latencyMs := firstByteDuration.Milliseconds()
	// Provider is considered healthy if the first byte arrived within 10 seconds
	providerHealthy := firstByteDuration > 0 && firstByteDuration <= 10*time.Second
	if !providerHealthy && !requestSucceeded {
		c.logWarn("Provider for model %s deemed unhealthy: No data received and first byte latency > 10s (or 0)", modelName)
	} else if !providerHealthy && requestSucceeded {
		c.logWarn("Provider for model %s deemed unhealthy despite success: First byte latency > 10s (%v)", modelName, firstByteDuration)
	}

	go func() {
		// Pass 'providerHealthy' status and latency to update function
		if err := provider.UpdateDbProvider(providerHealthy, latencyMs); err != nil {
			c.logError("Failed to update provider status: %v", err)
		} else {
			// Log both actual success and the health status passed
			c.logInfo("Provider status updated: healthy=%v (based on <=10s first byte), latency=%dms. Actual request success: %v",
				providerHealthy, latencyMs, requestSucceeded)
		}
	}()

	// Update API Key statistics using actual success
	go func() {
		inputBytes := int64(prompt.Len())
		outputBytes := total // Use the calculated total
		// Pass 'requestSucceeded' for API key stats
		if err := UpdateAiApiKeyStats(key.Key, inputBytes, outputBytes, requestSucceeded); err != nil {
			c.logError("Failed to update API key statistics: %v", err)
		} else {
			c.logInfo("API key statistics updated: key=%s, input=%d bytes, output=%d bytes, success=%v",
				utils.ShrinkString(key.Key, 8), inputBytes, outputBytes, requestSucceeded)
		}
	}()

	bandwidth := float64(0)
	if endDuration.Seconds() > 0 {
		bandwidth = float64(total) / endDuration.Seconds() / 1024
	}
	// Log actual success here too
	c.logInfo("Response completed (Success: %v), first byte duration: %v, end duration: %v, bandwidth: %.2fkbps, total bytes: %d",
		requestSucceeded, firstByteDuration, endDuration, bandwidth, total)

	writer.Close()
	utils.FlushWriter(conn)
	writer.Wait()
	conn.Close()
	c.logInfo("Connection closed for %s", conn.RemoteAddr())
}

// 新增函数: 处理 /v1/models 请求，返回所有可用的 model 列表
func (c *ServerConfig) serveModels(key *Key, conn net.Conn) {
	c.logInfo("Serving models list")

	// 定义模型信息结构，与 OpenAI API 格式一致
	type ModelMeta struct {
		ID      string `json:"id"`       // 模型ID（实际是 WrapperName）
		Object  string `json:"object"`   // 固定为 "model"
		Created int64  `json:"created"`  // 创建时间戳（Unix 时间）
		OwnedBy string `json:"owned_by"` // 模型所有者
	}

	// 构建响应数据结构
	type ModelsResponse struct {
		Object string       `json:"object"` // 固定为 "list"
		Data   []*ModelMeta `json:"data"`   // 使用指针切片与 ListChatModels 兼容
	}

	// 从 Entrypoints 中获取所有可用的模型
	modelNames := make([]string, 0, len(c.Entrypoints.providers))
	for modelName := range c.Entrypoints.providers {
		modelNames = append(modelNames, modelName)
	}

	// 如果没有模型，返回空列表
	if len(modelNames) == 0 {
		c.logWarn("No models available for listing")
	} else {
		c.logInfo("Found %d available models", len(modelNames))
	}

	// 构建响应对象
	response := ModelsResponse{
		Object: "list",
		Data:   make([]*ModelMeta, 0, len(modelNames)), // 使用指针切片
	}

	// 创建当前时间，用于 created 字段
	now := time.Now().Unix()

	// 为每个模型创建 ModelMeta
	for _, name := range modelNames {
		if key != nil {
			if _, ok := key.AllowedModels[name]; !ok {
				continue
			}
		}
		response.Data = append(response.Data, &ModelMeta{ // 使用指针
			ID:      name,
			Object:  "model",
			Created: now,
			OwnedBy: "library", // 改为 "library" 以匹配示例中的值
		})
	}

	// 序列化为 JSON
	responseJSON, err := json.Marshal(response)
	if err != nil {
		c.logError("Failed to marshal models response: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to encode models: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// 构建 HTTP 响应
	header := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: application/json; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n", len(responseJSON))

	// 发送响应
	conn.Write([]byte(header))
	conn.Write(responseJSON)
	c.logInfo("Models list response sent, %d bytes, models: %v", len(responseJSON), modelNames)
}

// serveIndexPage serves a simple HTML index page.
func (c *ServerConfig) serveIndexPage(conn net.Conn) {
	c.logInfo("Serving index page")

	var tmpl *template.Template
	var err error

	// Try to read template from filesystem first (consistent with portal.go)
	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/index.html",
		"templates/index.html",
		"../templates/index.html", // Added ../ for potential different execution paths
	); result != "" {
		rawTemp, ferr := os.ReadFile(result)
		if ferr != nil {
			c.logError("Failed to read index template from filesystem '%s': %v", result, ferr)
			// Fallback to embedded if reading fails
		} else {
			tmpl, err = template.New("index").Parse(string(rawTemp))
			if err != nil {
				c.logError("Failed to parse index template from filesystem: %v", err)
				// Fallback to embedded if parsing fails
			}
		}
	}

	// If filesystem read/parse failed or file not found, use embedded FS
	if tmpl == nil {
		tmpl, err = template.ParseFS(templatesFS, "templates/index.html")
		if err != nil {
			c.logError("Failed to parse embedded index template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to parse template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// Create a buffer to save rendered HTML
	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, nil) // Pass nil data as the template is static
	if err != nil {
		c.logError("Failed to execute index template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to render template: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Build the HTTP response
	response := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n%s", htmlBuffer.Len(), htmlBuffer.String())

	// Send the response
	_, err = conn.Write([]byte(response))
	if err != nil {
		c.logError("Failed to write index page response: %v", err)
	} else {
		c.logInfo("Index page response sent, %d bytes", len(response))
	}
}

func (c *ServerConfig) Serve(conn net.Conn) {
	c.logInfo("Received new connection request, source: %s", conn.RemoteAddr())
	defer conn.Close()
	reader := bufio.NewReader(conn)
	request, err := utils.ReadHTTPRequestFromBufioReader(reader)
	if err != nil {
		c.logError("Failed to read HTTP request: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	keys := make([]string, 0, len(request.Header))
	for k := range request.Header {
		keys = append(keys, k)
	}
	for _, k := range keys {
		if http.CanonicalHeaderKey(k) == k {
			continue
		}
		val, ok := request.Header[k]
		if ok {
			request.Header[http.CanonicalHeaderKey(k)] = val
			delete(request.Header, k)
		}
	}

	uriIns, err := url.ParseRequestURI(request.RequestURI)
	if err != nil {
		c.logError("Failed to parse request URI: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	c.logInfo("Request path: %s", uriIns.Path)
	requestRaw, err := utils.DumpHTTPRequest(request, true)
	if err != nil {
		c.logError("Failed to serialize HTTP request: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	c.logInfo("Raw request content:\n%s", string(requestRaw))

	switch {
	case strings.HasPrefix(uriIns.Path, "/forwarder/"):
		c.logInfo("Forwarder: registering with %s", uriIns.Path)
		c.serveForwarder(conn, requestRaw)
		return
	case strings.HasPrefix(uriIns.Path, "/v1/chat/completions"):
		c.logInfo("Processing chat completion request")
		c.serveChatCompletions(conn, requestRaw)
		return
	case strings.HasPrefix(uriIns.Path, "/v1/models"): // 新增：处理 /v1/models 请求
		c.logInfo("Processing models list request")
		key := c.getKeyFromRawRequest(requestRaw)
		c.serveModels(key, conn)
		return
	case strings.HasPrefix(uriIns.Path, "/portal"):
		c.HandlePortalRequest(conn, request, uriIns)
		return
	case uriIns.Path == "/":
		c.logInfo("Processing index page request for / ")
		c.serveIndexPage(conn)
		return
	case uriIns.Path == "/index":
		c.logInfo("Processing index page request for /index")
		c.serveIndexPage(conn)
		return
	case strings.HasPrefix(uriIns.Path, "/register/forward"):
		c.logInfo("Processing register forward request")
		fallthrough
	default:
		c.logError("Unknown request path: %s", uriIns.Path)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}
}
