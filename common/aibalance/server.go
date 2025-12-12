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

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/go-funk"

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

// PeekProvider returns a provider for the given model based on latency-weighted random selection
func (e *Entrypoints) PeekProvider(model string) *Provider {
	providers, ok := e.providers[model]
	if !ok || len(providers) == 0 {
		return nil
	}

	// 过滤出健康的提供者（延迟小于10秒）
	var healthyProviders []*Provider
	var totalWeight float64
	weights := make([]float64, 0, len(providers))

	for _, p := range providers {
		if p.DbProvider == nil {
			continue
		}

		// 检查提供者是否健康（延迟小于10秒）
		if p.DbProvider.IsHealthy && p.DbProvider.LastLatency > 0 && p.DbProvider.LastLatency < 10000 {
			healthyProviders = append(healthyProviders, p)
			// 使用延迟的倒数作为权重，延迟越低权重越高
			weight := 1.0 / float64(p.DbProvider.LastLatency)
			weights = append(weights, weight)
			totalWeight += weight
		}
	}

	// 如果没有健康的提供者，返回 nil
	if len(healthyProviders) == 0 {
		return nil
	}

	// 如果只有一个健康的提供者，直接返回
	if len(healthyProviders) == 1 {
		return healthyProviders[0]
	}

	// 生成随机数
	r := utils.RandFloat64() * totalWeight

	// 根据权重选择提供者
	var cumulativeWeight float64
	for i, weight := range weights {
		cumulativeWeight += weight
		if r <= cumulativeWeight {
			return healthyProviders[i]
		}
	}

	// 如果由于浮点数精度问题没有选中任何提供者，返回最后一个
	return healthyProviders[len(healthyProviders)-1]
}

// PeekOrderedProviders returns providers for the given model in random order
// Only returns providers with latency < 10s, randomly shuffled
func (e *Entrypoints) PeekOrderedProviders(model string) []*Provider {
	providers, ok := e.providers[model]
	if !ok || len(providers) == 0 {
		log.Debugf("No providers found for model: %s", model)
		return nil
	}

	log.Infof("PeekOrderedProviders for model %s: found %d providers", model, len(providers))

	// 如果只有一个提供者，无论其健康状况如何，都直接返回
	if len(providers) == 1 {
		log.Infof("Only one provider found for model %s, returning it directly.", model)
		return providers
	}

	// 过滤出延迟小于10秒的提供者
	var validProviders []*Provider
	for _, p := range providers {
		if p.DbProvider == nil {
			log.Debugf("Provider %s skipped (no DbProvider)", p.TypeName)
			continue
		}

		// 只保留延迟小于10秒的提供者
		if p.DbProvider.LastLatency > 0 && p.DbProvider.LastLatency < 10000 {
			validProviders = append(validProviders, p)
			log.Debugf("Provider %s accepted (latency: %dms, healthy: %v)",
				p.TypeName, p.DbProvider.LastLatency, p.DbProvider.IsHealthy)
		} else {
			log.Infof("Provider %s filtered out (latency: %dms >= 10s or no latency data)",
				p.TypeName, p.DbProvider.LastLatency)
		}
	}

	if len(validProviders) == 0 {
		log.Debugf("No valid providers found for model %s (all have latency >= 10s)", model)
		return nil
	}

	log.Debugf("Found %d valid providers (latency < 10s) for model %s", len(validProviders), model)

	// 使用 Fisher-Yates 洗牌算法完全随机打乱
	shuffledProviders := make([]*Provider, len(validProviders))
	copy(shuffledProviders, validProviders)

	for i := len(shuffledProviders) - 1; i > 0; i-- {
		j := int(utils.RandFloat64() * float64(i+1))
		shuffledProviders[i], shuffledProviders[j] = shuffledProviders[j], shuffledProviders[i]
	}

	// 输出随机排序结果
	log.Debugf("Randomly shuffled providers for model %s:", model)
	for i, p := range shuffledProviders {
		log.Debugf("  %d. %s (latency: %dms, healthy: %v)",
			i+1, p.TypeName, p.DbProvider.LastLatency, p.DbProvider.IsHealthy)
	}

	return shuffledProviders
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

	var bodyIns aispec.ChatMessage
	err := json.Unmarshal(body, &bodyIns)
	if err != nil {
		c.logError("Failed to parse request body: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	stream := bodyIns.Stream
	log.Infof("user require stream flag: %v", stream)

	modelName := bodyIns.Model
	c.logInfo("Requested model: %s", modelName)
	isFreeModel := strings.HasSuffix(modelName, "-free")
	if isFreeModel {
		c.logInfo("Request is for a free model, skipping key verification.")
	}

	var key *Key
	var apiKeyForStat string

	if isFreeModel {
		apiKeyForStat = "free-user"
	} else {
		value := strings.TrimPrefix(auth, "Bearer ")
		c.logInfo("Extracted key from authentication info: %s", value)
		if value == "" {
			c.logError("No valid authentication info provided")
			conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
			return
		}

		var ok bool
		key, ok = c.Keys.Get(value)
		if !ok {
			c.logError("No matching key configuration found: %s", value)
			conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
			return
		}
		apiKeyForStat = key.Key
		c.logInfo("Successfully verified key: %s", key.Key)

		// Authorization check
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
	}

	var prompt bytes.Buffer
	var imageContent []*aispec.ChatContent
	for _, message := range bodyIns.Messages {
		switch ret := message.Content.(type) {
		case string:
			log.Infof("Received text content: %s", utils.ShrinkString(ret, 200))
			prompt.Write([]byte(ret))
			//contents = append(contents, aispec.NewUserChatContentText(ret))
		default:
			handleItem := func(element any) {
				if utils.IsMap(element) {
					// handle images
					generalMap := utils.InterfaceToGeneralMap(element)
					typeName := utils.MapGetString(generalMap, `type`)
					switch typeName {
					case "image_url":
						txt := utils.MapGetString(utils.MapGetMapRaw(generalMap, `image_url`), "url")
						log.Infof("meet image_url.url with: %#v", utils.ShrinkString(txt, 200))
						imageContent = append(imageContent, aispec.NewUserChatContentImageUrl(txt))
					case "text":
						txt := utils.MapGetString(generalMap, "text") + "\n"
						log.Infof("meet text with: %#v", utils.ShrinkString(txt, 200))
						prompt.Write([]byte(txt))
					default:
						log.Infof("unknown type: %s with %v", typeName, spew.Sdump(ret))
					}
				} else {
					log.Infof("Received unknown content: %s", utils.ShrinkString(element, 300))
					prompt.Write(utils.InterfaceToBytes(element))
				}
			}
			if funk.IsIteratee(ret) {
				funk.ForEach(ret, func(i any) {
					handleItem(i)
				})
			} else {
				log.Infof("Received unknown content: %s", utils.ShrinkString(ret, 300))
				prompt.Write(utils.InterfaceToBytes(ret))
			}
		}
	}

	if len(imageContent) == 0 && prompt.Len() <= 0 {
		c.logError("Prompt is empty")
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nX-Reason: empty prompt\r\n\r\n"))
		return
	}

	c.logInfo("Built prompt length: %d with image content: %d", prompt.Len(), len(imageContent))

	// model, ok := c.Models.Get(modelName)
	// if !ok {
	// 	c.logError("No model configuration found: %s", modelName)
	// 	conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
	// 	return
	// }

	// c.logInfo("Key[%v] requesting model %s, starting to forward request", apiKeyForStat, modelName)
	// _ = model

	// 使用 PeekOrderedProviders 获取按优先级排序的提供者列表
	providers := c.Entrypoints.PeekOrderedProviders(modelName)
	if len(providers) == 0 {
		// 如果找不到，尝试从数据库重新加载
		c.logWarn("No valid providers found for model %s, trying to reload from database...", modelName)
		if err := LoadProvidersFromDatabase(c); err != nil {
			c.logError("Failed to reload providers from database: %v", err)
		} else {
			c.logInfo("Successfully reloaded providers from database, retrying to find providers.")
			providers = c.Entrypoints.PeekOrderedProviders(modelName)
		}
	}

	if len(providers) == 0 {
		c.logError("No valid providers found for model %s (all providers have latency >= 10s)", modelName)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 404 Not Found\r\nX-Reason: no valid provider found for %v, all providers have high latency\r\n\r\n", modelName)))
		return
	}

	c.logInfo("Found %d valid providers for model %s, trying in order", len(providers), modelName)

	// 尝试每个提供者，直到有一个成功
	var successfulProvider *Provider
	var lastError error
	for i, provider := range providers {
		c.logInfo("Trying provider %d/%d for model %s: %s", i+1, len(providers), modelName, provider.TypeName)

		sendHeaderOnce := sync.Once{}
		sendHeader := func() {
			c.logInfo("Successfully obtained AI client, starting to send response header")
			var header = "HTTP/1.1 200 OK\r\n" +
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

		writer := NewChatJSONChunkWriter(conn, apiKeyForStat, modelName)
		client, err := provider.GetAIClientWithImages(
			imageContent,
			func(reader io.Reader) {
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
			},
		)
		if err != nil {
			c.logError("Failed to get AI client from provider %s: %v", provider.TypeName, err)
			lastError = err
			continue // 尝试下一个提供者
		}

		// 启动 AI 聊天请求
		chatCompleted := make(chan error, 1)
		go func() {
			c.logInfo("start to call ai chat interface with prompt len: %d", prompt.Len())
			finalMsg, err := client.Chat(prompt.String())
			if err != nil {
				c.logError("AI chat interface call failed: %v", err)
				chatCompleted <- err
				return
			}
			c.logInfo("AI chat interface call completed, final: %v.", utils.ShrinkString(finalMsg, 100))
			chatCompleted <- nil
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

		// 检查聊天请求是否成功完成
		select {
		case chatErr := <-chatCompleted:
			if chatErr != nil {
				c.logError("Provider %s chat failed: %v", provider.TypeName, chatErr)
				lastError = chatErr
				// 更新失败的提供者状态
				latencyMs := firstByteDuration.Milliseconds()
				go func() {
					if err := provider.UpdateDbProvider(false, latencyMs); err != nil {
						c.logError("Failed to update failed provider status: %v", err)
					}
				}()
				continue // 尝试下一个提供者
			}
		default:
			// 如果聊天还在进行中，检查是否有数据传输
			if !requestSucceeded {
				c.logWarn("No data received from provider %s for model %s", provider.TypeName, modelName)
				lastError = fmt.Errorf("no data received from provider")
				// 更新失败的提供者状态
				latencyMs := firstByteDuration.Milliseconds()
				go func() {
					if err := provider.UpdateDbProvider(false, latencyMs); err != nil {
						c.logError("Failed to update failed provider status: %v", err)
					}
				}()
				continue // 尝试下一个提供者
			}
		}

		// 如果到达这里，说明当前提供者成功了
		successfulProvider = provider
		c.logInfo("Provider %s successfully handled the request for model %s", provider.TypeName, modelName)

		// Update successful provider status
		latencyMs := firstByteDuration.Milliseconds()
		providerHealthy := firstByteDuration > 0 && firstByteDuration <= 10*time.Second
		go func() {
			if err := provider.UpdateDbProvider(providerHealthy, latencyMs); err != nil {
				c.logError("Failed to update provider status: %v", err)
			} else {
				c.logInfo("Provider status updated: healthy=%v (based on <=10s first byte), latency=%dms. Actual request success: %v",
					providerHealthy, latencyMs, requestSucceeded)
			}
		}()

		// Update API Key statistics using actual success
		if !isFreeModel {
			go func() {
				inputBytes := int64(prompt.Len())
				outputBytes := total
				if err := UpdateAiApiKeyStats(key.Key, inputBytes, outputBytes, requestSucceeded); err != nil {
					c.logError("Failed to update API key statistics: %v", err)
				} else {
					c.logInfo("API key statistics updated: key=%s, input=%d bytes, output=%d bytes, success=%v",
						utils.ShrinkString(key.Key, 8), inputBytes, outputBytes, requestSucceeded)
				}
			}()
		}

		bandwidth := float64(0)
		if endDuration.Seconds() > 0 {
			bandwidth = float64(total) / endDuration.Seconds() / 1024
		}
		c.logInfo("Response completed (Success: %v), first byte duration: %v, end duration: %v, bandwidth: %.2fkbps, total bytes: %d",
			requestSucceeded, firstByteDuration, endDuration, bandwidth, total)

		writer.Close()
		utils.FlushWriter(conn)
		writer.Wait()
		break // 成功处理，退出循环
	}

	// 如果所有提供者都失败了
	if successfulProvider == nil {
		c.logError("All providers failed for model %s, last error: %v", modelName, lastError)
		errorMsg := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nX-Reason: all providers failed for %v, last error: %v\r\n\r\n", modelName, lastError)
		conn.Write([]byte(errorMsg))
		return
	}

	conn.Close()
	c.logInfo("Connection closed for %s", conn.RemoteAddr())
}

// serveEmbeddings handles embedding requests
func (c *ServerConfig) serveEmbeddings(conn net.Conn, rawPacket []byte) {
	c.logInfo("Starting to handle new embedding request")

	// Extract authorization header
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

	// Parse request body
	type EmbeddingRequest struct {
		Input          string `json:"input"`
		Model          string `json:"model"`
		EncodingFormat string `json:"encoding_format,omitempty"`
	}

	var reqBody EmbeddingRequest
	err := json.Unmarshal(body, &reqBody)
	if err != nil {
		c.logError("Failed to parse request body: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	modelName := reqBody.Model
	inputText := reqBody.Input
	c.logInfo("Requested embedding model: %s, input length: %d", modelName, len(inputText))

	if inputText == "" {
		c.logError("Input text is empty")
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nX-Reason: empty input\r\n\r\n"))
		return
	}

	// Check if it's a free model
	isFreeModel := strings.HasSuffix(modelName, "-free")
	if isFreeModel {
		c.logInfo("Request is for a free embedding model, skipping key verification.")
	}

	var key *Key

	if !isFreeModel {
		value := strings.TrimPrefix(auth, "Bearer ")
		c.logInfo("Extracted key from authentication info: %s", value)
		if value == "" {
			c.logError("No valid authentication info provided")
			conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
			return
		}

		var ok bool
		key, ok = c.Keys.Get(value)
		if !ok {
			c.logError("No matching key configuration found: %s", value)
			conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
			return
		}
		c.logInfo("Successfully verified key: %s", key.Key)

		// Authorization check
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
	}

	// Get providers for the model
	providers := c.Entrypoints.PeekOrderedProviders(modelName)
	if len(providers) == 0 {
		// Try to reload from database
		c.logWarn("No valid providers found for model %s, trying to reload from database...", modelName)
		if err := LoadProvidersFromDatabase(c); err != nil {
			c.logError("Failed to reload providers from database: %v", err)
		} else {
			c.logInfo("Successfully reloaded providers from database, retrying to find providers.")
			providers = c.Entrypoints.PeekOrderedProviders(modelName)
		}
	}

	if len(providers) == 0 {
		c.logError("No valid providers found for embedding model %s", modelName)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 404 Not Found\r\nX-Reason: no valid provider found for %v\r\n\r\n", modelName)))
		return
	}

	c.logInfo("Found %d valid providers for embedding model %s, trying in order", len(providers), modelName)

	// Try each provider until one succeeds
	var successfulProvider *Provider
	var lastError error
	var embeddingResult []float32

	for i, provider := range providers {
		c.logInfo("Trying provider %d/%d for embedding model %s: %s", i+1, len(providers), modelName, provider.TypeName)

		start := time.Now()

		// Get embedding client
		embClient, err := provider.GetEmbeddingClient()
		if err != nil {
			c.logError("Failed to get embedding client from provider %s: %v", provider.TypeName, err)
			lastError = err
			continue
		}

		// Call embedding
		vectors, err := embClient.Embedding(inputText)
		if err != nil {
			c.logError("Embedding call failed for provider %s: %v", provider.TypeName, err)
			lastError = err
			latencyMs := time.Since(start).Milliseconds()
			go func() {
				if err := provider.UpdateDbProvider(false, latencyMs); err != nil {
					c.logError("Failed to update failed provider status: %v", err)
				}
			}()
			continue
		}

		// Success
		embeddingResult = vectors
		successfulProvider = provider
		latencyMs := time.Since(start).Milliseconds()
		c.logInfo("Provider %s successfully generated embedding (dimension: %d, latency: %dms)", provider.TypeName, len(vectors), latencyMs)

		// Update provider status
		providerHealthy := latencyMs < 10000
		go func() {
			if err := provider.UpdateDbProvider(providerHealthy, latencyMs); err != nil {
				c.logError("Failed to update provider status: %v", err)
			} else {
				c.logInfo("Provider status updated: healthy=%v, latency=%dms", providerHealthy, latencyMs)
			}
		}()

		// Update API Key statistics
		if !isFreeModel {
			go func() {
				inputBytes := int64(len(inputText))
				outputBytes := int64(len(vectors) * 4) // float32 = 4 bytes
				if err := UpdateAiApiKeyStats(key.Key, inputBytes, outputBytes, true); err != nil {
					c.logError("Failed to update API key statistics: %v", err)
				} else {
					c.logInfo("API key statistics updated: key=%s, input=%d bytes, output=%d bytes",
						utils.ShrinkString(key.Key, 8), inputBytes, outputBytes)
				}
			}()
		}

		break // Success, exit loop
	}

	// If all providers failed
	if successfulProvider == nil {
		c.logError("All providers failed for embedding model %s, last error: %v", modelName, lastError)
		errorMsg := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nX-Reason: all providers failed for %v, last error: %v\r\n\r\n", modelName, lastError)
		conn.Write([]byte(errorMsg))
		return
	}

	// Build response in OpenAI format
	type EmbeddingData struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}

	type EmbeddingResponse struct {
		Object string          `json:"object"`
		Data   []EmbeddingData `json:"data"`
		Model  string          `json:"model"`
		Usage  struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}

	response := EmbeddingResponse{
		Object: "list",
		Data: []EmbeddingData{
			{
				Object:    "embedding",
				Embedding: embeddingResult,
				Index:     0,
			},
		},
		Model: modelName,
	}
	response.Usage.PromptTokens = len(inputText)
	response.Usage.TotalTokens = len(inputText)

	// Marshal response
	responseJSON, err := json.Marshal(response)
	if err != nil {
		c.logError("Failed to marshal embedding response: %v", err)
		conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		return
	}

	// Send response
	header := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: application/json; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n", len(responseJSON))

	conn.Write([]byte(header))
	conn.Write(responseJSON)
	c.logInfo("Embedding response sent successfully, %d bytes", len(responseJSON))
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
		// 免费模型始终对所有用户可见
		// 对于非免费模型，如果提供了 key，则检查权限
		isFreeModel := strings.HasSuffix(name, "-free")
		if key != nil {
			if _, ok := key.AllowedModels[name]; !ok && !isFreeModel {
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
	//c.logInfo("Received new connection request, source: %s", conn.RemoteAddr())
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// Support HTTP/1.1 Keep-Alive (enabled by default)
	// Handle multiple requests on the same connection
	for {
		// Set read deadline to avoid hanging connections
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		request, err := utils.ReadHTTPRequestFromBufioReader(reader)
		if err != nil {
			// Connection closed or timeout, this is normal for keep-alive
			if err == io.EOF {
				return
			}
			c.logError("Failed to read HTTP request: %v", err)
			return
		}

		// HTTP/1.1 defaults to keep-alive, only close if explicitly requested
		shouldClose := false
		connectionHeader := request.Header.Get("Connection")

		if request.ProtoMajor == 1 && request.ProtoMinor == 0 {
			// HTTP/1.0 defaults to close unless keep-alive is specified
			if connectionHeader == "" || !strings.EqualFold(connectionHeader, "keep-alive") {
				shouldClose = true
			}
		} else {
			// HTTP/1.1 defaults to keep-alive
			// Only close if explicitly set to "close"
			if strings.EqualFold(connectionHeader, "close") {
				shouldClose = true
			}
			// Handle "Connection: upgrade" from nginx proxy
			// If Connection is "upgrade" but no Upgrade header is present,
			// this is not a real WebSocket upgrade request, so close the connection
			// to avoid pending issues with nginx proxy
			if strings.EqualFold(connectionHeader, "upgrade") {
				upgradeHeader := request.Header.Get("Upgrade")
				if upgradeHeader == "" {
					// Not a real upgrade request, close connection after response
					shouldClose = true
				}
			}
		}

		// Process the request
		c.serveRequest(conn, request, shouldClose)

		// If client requested connection close, break the loop
		if shouldClose {
			return
		}
	}
}

func (c *ServerConfig) serveRequest(conn net.Conn, request *http.Request, shouldClose bool) {
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
		c.writeResponse(conn, "HTTP/1.1 400 Bad Request\r\n\r\n", shouldClose)
		return
	}

	//c.logInfo("Request path: %s", uriIns.Path)
	requestRaw, err := utils.DumpHTTPRequest(request, true)
	if err != nil {
		c.logError("Failed to serialize HTTP request: %v", err)
		c.writeResponse(conn, "HTTP/1.1 400 Bad Request\r\n\r\n", shouldClose)
		return
	}

	//c.logInfo("Raw request content:\n%s", string(requestRaw))

	switch {
	case strings.HasPrefix(uriIns.Path, "/forwarder/"):
		//c.logInfo("Forwarder: registering with %s", uriIns.Path)
		c.serveForwarder(conn, requestRaw)
		return
	case strings.HasPrefix(uriIns.Path, "/v1/chat/completions"):
		c.serveChatCompletions(conn, requestRaw)
		return
	case strings.HasPrefix(uriIns.Path, "/v1/embeddings"):
		c.serveEmbeddings(conn, requestRaw)
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
		c.writeResponse(conn, "HTTP/1.1 404 Not Found\r\n\r\n", shouldClose)
		return
	}
}

// writeResponse writes a response with appropriate Connection header for keep-alive support
func (c *ServerConfig) writeResponse(conn net.Conn, response string, shouldClose bool) {
	if shouldClose {
		// Add Connection: close header if not already present
		if !strings.Contains(response, "Connection:") {
			lines := strings.Split(response, "\r\n")
			if len(lines) > 0 {
				var builder strings.Builder
				builder.WriteString(lines[0])
				builder.WriteString("\r\n")
				builder.WriteString("Connection: close\r\n")
				for i := 1; i < len(lines); i++ {
					builder.WriteString(lines[i])
					if i < len(lines)-1 {
						builder.WriteString("\r\n")
					}
				}
				conn.Write([]byte(builder.String()))
				return
			}
		}
	}
	conn.Write([]byte(response))
}
