package aibalance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"

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
	// 返回第一个 provider
	return providers[0]
}

// GetAllProviders returns all providers for the given model
func (e *Entrypoints) GetAllProviders(model string) []*Provider {
	return e.providers[model]
}

// ServerConfig represents the server configuration
type ServerConfig struct {
	Keys             *KeyManager
	KeyAllowedModels *KeyAllowedModels
	Models           *ModelManager
	Entrypoints      *Entrypoints
	Logging          LogLevel
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
	writer := NewChatJSONChunkWriter(conn, key.Key, modelName)
	c.logInfo("Starting to call AI chat interface")

	sendHeaderOnce := sync.Once{}
	sendHeader := func() {
		c.logInfo("Successfully obtained AI client, starting to send response header")
		header := "HTTP/1.1 200 OK\r\n" +
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
	_, _ = reasonWriter, outputWriter

	// Handle reason stream
	go func() {
		defer func() {
			c.logInfo("Finished forwarding AI response stream(reason)")
			wg.Done()
		}()
		c.logInfo("Start to handle reason mirror stream")
		n, err := io.Copy(reasonWriter, rr)
		if err != nil {
			c.logError("Failed to copy reason stream: %v", err)
		}
		c.logInfo("Reason stream copy completed, bytes: %d", n)
	}()

	// Handle output stream
	go func() {
		defer func() {
			c.logInfo("Finished forwarding AI response stream(output)")
			wg.Done()
		}()
		c.logInfo("Start to handle output mirror stream")
		n, err := io.Copy(outputWriter, pr)
		if err != nil {
			c.logError("Failed to copy output stream: %v", err)
		}
		c.logInfo("Output stream copy completed, bytes: %d", n)
	}()

	// Wait for all stream processing to complete
	wg.Wait()

	c.logInfo("Response completed")
	writer.Close()
	conn.Close()
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
	case strings.HasPrefix(uriIns.Path, "/v1/chat/completions"):
		c.logInfo("Processing chat completion request")
		c.serveChatCompletions(conn, requestRaw)
		return
	case uriIns.Path == "/register/forward":
		c.logInfo("Processing register forward request")
		fallthrough
	default:
		c.logError("Unknown request path: %s", uriIns.Path)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return
	}
}
