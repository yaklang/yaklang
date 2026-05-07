package aibalance

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ConfigProvider is the provider structure for configuration files
type ConfigProvider struct {
	ModelName   string   `yaml:"model_name" json:"model_name"`
	TypeName    string   `yaml:"type_name" json:"type_name"`
	DomainOrURL string   `yaml:"domain_or_url" json:"domain_or_url"`
	APIKey      string   `yaml:"api_key" json:"api_key"`
	Keys        []string `yaml:"keys" json:"keys"`
	KeyFile     string   `yaml:"key_file" json:"key_file"`
	NoHTTPS     bool     `yaml:"no_https" json:"no_https"`
	// works for qwen3
	OptionalAllowReason  string `yaml:"optional_allow_reason,omitempty" json:"optional_allow_reason,omitempty"`
	OptionalReasonBudget int    `yaml:"optional_reason_budget,omitempty" json:"optional_reason_budget,omitempty"`

	// ActiveCacheControl 让该 provider 主动管理 cache_control:
	// 客户端无 cc 时给最末 system 注入 ephemeral cc, 客户端自带 cc 时 pass-through。
	// 关键词: ConfigProvider ActiveCacheControl, yaml/json 双 tag, ephemeral baseline
	ActiveCacheControl bool `yaml:"active_cache_control,omitempty" json:"active_cache_control,omitempty"`
}

// Provider is the provider structure for actual API calls
type Provider struct {
	ModelName    string `json:"model_name"`
	TypeName     string `json:"type_name"`
	ProviderMode string `json:"provider_mode"` // "chat" or "embedding"
	DomainOrURL  string `json:"domain_or_url"`
	APIKey       string `json:"api_key"`
	NoHTTPS      bool   `json:"no_https"`
	// works for qwen3
	OptionalAllowReason  string `json:"optional_allow_reason,omitempty"`
	OptionalReasonBudget int    `json:"optional_reason_budget,omitempty"`

	// ActiveCacheControl 控制是否走主动 cache_control 注入路径,
	// 由 RewriteMessagesForProviderInstance 在透传 messages 给上游前消费。
	// 关键词: Provider ActiveCacheControl, 主动 cache_control 注入开关
	ActiveCacheControl bool `json:"active_cache_control"`

	// External display name for users, usually an alias for the model
	WrapperName string `json:"wrapper_name"`

	// Corresponding AiProvider object in the database
	DbProvider *schema.AiProvider `json:"-"` // json:"-" means this field won't be serialized to JSON

	// Mutex to protect concurrent updates
	mutex sync.Mutex `json:"-"`
}

// toProvider converts ConfigProvider to Provider (private method)
func (cp *ConfigProvider) toProvider(apiKey string) *Provider {
	// Ensure TypeName is not empty
	if cp.TypeName == "" {
		log.Errorf("Provider type name cannot be empty")
		return nil
	}

	// Ensure DomainOrURL is valid (required for most providers)
	if cp.DomainOrURL == "" && cp.TypeName != "ollama" {
		// log.Errorf("Provider domain or URL cannot be empty for type: %s", cp.TypeName)
		// return nil
	}

	return &Provider{
		ModelName:            cp.ModelName,
		TypeName:             cp.TypeName,
		DomainOrURL:          cp.DomainOrURL,
		APIKey:               apiKey,
		NoHTTPS:              cp.NoHTTPS,
		OptionalAllowReason:  cp.OptionalAllowReason,
		OptionalReasonBudget: cp.OptionalReasonBudget,
		ActiveCacheControl:   cp.ActiveCacheControl,
		// WrapperName is initially empty, set by external
		WrapperName: "",
	}
}

// ToProviders converts ConfigProvider to multiple Providers
func (cp *ConfigProvider) ToProviders() []*Provider {
	providers := make([]*Provider, 0)

	// Validate required fields
	if cp.TypeName == "" {
		log.Errorf("Provider type name cannot be empty")
		return nil
	}

	allKeys := cp.GetAllKeys()

	// If no keys available, only use default provider in certain cases
	if len(allKeys) == 0 {
		// Some provider types may not require API keys (e.g., local models or open source models)
		if cp.TypeName == "ollama" {
			// For providers that don't need API keys, use default provider
			providers = append(providers, cp.toProvider(""))
			return providers
		}

		log.Warnf("No API keys available for provider type: %s", cp.TypeName)
		return nil // No available keys, return empty
	}

	// Create a new provider for each key
	for _, key := range allKeys {
		log.Infof("ToProviders: type: %v, model: %s, key: %s", cp.TypeName, cp.ModelName, utils.ShrinkString(key, 8))
		provider := cp.toProvider(key)
		if provider != nil {
			providers = append(providers, provider)
		}
	}

	return providers
}

// GetAllKeys gets all available keys
func (cp *ConfigProvider) GetAllKeys() []string {
	var allKeys []string

	// Check directly configured API key
	if cp.APIKey != "" {
		allKeys = append(allKeys, cp.APIKey)
	}

	// Check key list
	if len(cp.Keys) > 0 {
		allKeys = append(allKeys, cp.Keys...)
	}

	// Check KeyFile (file containing multiple keys)
	if cp.KeyFile != "" {
		// Try to read the file
		data, err := os.ReadFile(cp.KeyFile)
		if err != nil {
			log.Errorf("Failed to read key file %s: %v", cp.KeyFile, err)
		} else {
			// Split by lines, one key per line
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					allKeys = append(allKeys, line)
				}
			}
		}
	}

	return allKeys
}

// GetAIClientWithImagesAndTools 旧路径，构造时把 prompt 拍成单条 user 消息 + image content 数组。
// 仍保留用于 healthy check / handle_providers 等"无 messages 数组"的连通性测试。
// 正式 chat completions 转发路径已统一改用 GetAIClientWithRawMessages，请勿在
// 新代码中调用此函数承载用户 prompt。
//
// Deprecated: 优先使用 GetAIClientWithRawMessages 透传完整 messages 数组。
// 关键词: GetAIClientWithImagesAndTools, deprecated, 仅 healthy check 使用
func (p *Provider) GetAIClientWithImagesAndTools(imageContents []*aispec.ChatContent, tools []aispec.Tool, toolChoice any, enableThinking bool, onStream, onReasonStream func(reader io.Reader), onToolCall func([]*aispec.ToolCall)) (aispec.AIClient, error) {
	log.Infof("GetAIClient: type: %s, domain: %s, key: %s, model: %s, no_https: %v, tools: %d", p.TypeName, p.DomainOrURL, utils.ShrinkString(p.APIKey, 8), p.ModelName, p.NoHTTPS, len(tools))

	var images []any
	for _, content := range imageContents {
		if content.Type == "image_url" {
			images = append(images, content)
		}
	}
	var opts []aispec.AIConfigOption
	opts = append(
		opts,
		aispec.WithType(p.TypeName),
		aispec.WithChatImageContent(images...),
		aispec.WithTimeout(10),
		aispec.WithNoHTTPS(p.NoHTTPS),
		aispec.WithAPIKey(p.APIKey),
		aispec.WithModel(p.ModelName),
		// NOTE: Do NOT create goroutines here!
		// The stream handler is already called in a separate goroutine by the AI SDK.
		// Creating additional goroutines here causes goroutine leaks because they
		// block on reader.Read() and never exit when the connection is closed.
		aispec.WithStreamHandler(func(reader io.Reader) {
			if onStream != nil {
				onStream(reader)
			} else {
				io.Copy(os.Stdout, reader)
			}
		}),
		aispec.WithReasonStreamHandler(func(reader io.Reader) {
			if onReasonStream != nil {
				onReasonStream(reader)
			} else {
				io.Copy(os.Stdout, reader)
			}
		}),
	)

	shouldEnableThinking := enableThinking
	forceDisableThinking := false
	if p.OptionalAllowReason != "" {
		switch strings.ToLower(strings.TrimSpace(p.OptionalAllowReason)) {
		case "true", "yes", "1", "enable", "on":
			shouldEnableThinking = true
		case "false", "no", "0", "disable", "off":
			shouldEnableThinking = false
			forceDisableThinking = true
		}
	}
	if shouldEnableThinking {
		log.Infof("GetAIClient: enable_thinking=true for type=%s (OptionalAllowReason=%q, clientRequest=%v)", p.TypeName, p.OptionalAllowReason, enableThinking)
		opts = append(opts, aispec.WithEnableThinking(true))
	} else if forceDisableThinking {
		log.Infof("GetAIClient: enable_thinking=false (force disabled) for type=%s (OptionalAllowReason=%q)", p.TypeName, p.OptionalAllowReason)
		opts = append(opts, aispec.WithEnableThinking(false))
	}

	// Add tool call callback if provided
	// This enables forwarding tool_calls from AI provider to the client
	if onToolCall != nil {
		opts = append(opts, aispec.WithToolCallCallback(onToolCall))
	}

	// Add tools and tool_choice if provided
	if len(tools) > 0 {
		opts = append(opts, aispec.WithTools(tools))
	}
	if toolChoice != nil {
		opts = append(opts, aispec.WithToolChoice(toolChoice))
	}

	if target := strings.TrimSpace(p.DomainOrURL); target != "" {
		if utils.IsHttpOrHttpsUrl(target) {
			opts = append(opts, aispec.WithBaseURL(target))
		} else {
			opts = append(opts, aispec.WithDomain(target))
		}
	}
	client := ai.GetAI(p.TypeName, opts...)
	if utils.IsNil(client) || client == nil {
		return nil, errors.New("failed to get ai client, no such type: " + p.TypeName)
	}
	return client, nil
}

// GetAIClientWithImages 仅保留以兼容旧测试与 healthy check。
// Deprecated: 优先使用 GetAIClientWithRawMessages 透传完整 messages 数组。
// 关键词: GetAIClientWithImages, deprecated
func (p *Provider) GetAIClientWithImages(imageContents []*aispec.ChatContent, enableThinking bool, onStream, onReasonStream func(reader io.Reader), onToolCall func([]*aispec.ToolCall)) (aispec.AIClient, error) {
	return p.GetAIClientWithImagesAndTools(imageContents, nil, nil, enableThinking, onStream, onReasonStream, onToolCall)
}

// GetAIClientWithRawMessages 构造一个携带"完整原始 messages 数组"的 AI 客户端，
// 用于 aibalance 中转层把客户端 messages 一字不差地透传到上游 LLM，最大化
// 隐式缓存的前缀字节命中率。
//
// 与 GetAIClientWithImagesAndTools 的区别：
//   - 不再单独提取 image_url 注入 WithChatImageContent —— 因为 image_url 已经
//     直接放在 messages 内的 content 数组中，由 RawMessages 统一携带；
//   - 通过 aispec.WithRawMessages(messages) 把整个 messages 数组写入 AIConfig，
//     gateway.Chat(s) 会再透传到 ChatBaseContext.RawMessages，
//     由 chatBaseChatCompletions 直接使用，跳过单 user 包装。
//
// 旧的 GetAIClientWithImagesAndTools/GetAIClientWithImages 保留不变，便于
// 不需要 messages 完整透传的旧调用方继续使用。
//
// 关键词: GetAIClientWithRawMessages, aibalance messages 透传, 隐式缓存前缀稳定, usage 回调
//
// onUsage 是新增参数：当上游 LLM 的 SSE 末帧带 usage 字段时（例如
// stream_options.include_usage=true 触发的 token 用量帧），aispec 会回调 onUsage。
// aibalance server 把它接到 chatJSONChunkWriter.WriteUsage，再由 writer.Close
// 在 [DONE] 帧之前往下游客户端发一帧 choices=[] + usage={...}，达成
// "上游 cached_tokens -> 下游客户端可见"的端到端透传。
func (p *Provider) GetAIClientWithRawMessages(messages []aispec.ChatDetail, tools []aispec.Tool, toolChoice any, enableThinking bool, onStream, onReasonStream func(reader io.Reader), onToolCall func([]*aispec.ToolCall), onUsage func(*aispec.ChatUsage)) (aispec.AIClient, error) {
	log.Infof("GetAIClientWithRawMessages: type: %s, domain: %s, key: %s, model: %s, no_https: %v, messages: %d, tools: %d", p.TypeName, p.DomainOrURL, utils.ShrinkString(p.APIKey, 8), p.ModelName, p.NoHTTPS, len(messages), len(tools))

	var opts []aispec.AIConfigOption
	opts = append(
		opts,
		aispec.WithType(p.TypeName),
		aispec.WithTimeout(10),
		aispec.WithNoHTTPS(p.NoHTTPS),
		aispec.WithAPIKey(p.APIKey),
		aispec.WithModel(p.ModelName),
		// RawMessages 透传：让 gateway.Chat(s) 把整个 messages 数组带到 ChatBase
		aispec.WithRawMessages(messages),
		aispec.WithStreamHandler(func(reader io.Reader) {
			if onStream != nil {
				onStream(reader)
			} else {
				io.Copy(os.Stdout, reader)
			}
		}),
		aispec.WithReasonStreamHandler(func(reader io.Reader) {
			if onReasonStream != nil {
				onReasonStream(reader)
			} else {
				io.Copy(os.Stdout, reader)
			}
		}),
	)
	if onUsage != nil {
		opts = append(opts, aispec.WithUsageCallback(onUsage))
	}

	shouldEnableThinking := enableThinking
	forceDisableThinking := false
	if p.OptionalAllowReason != "" {
		switch strings.ToLower(strings.TrimSpace(p.OptionalAllowReason)) {
		case "true", "yes", "1", "enable", "on":
			shouldEnableThinking = true
		case "false", "no", "0", "disable", "off":
			shouldEnableThinking = false
			forceDisableThinking = true
		}
	}
	if shouldEnableThinking {
		log.Infof("GetAIClientWithRawMessages: enable_thinking=true for type=%s (OptionalAllowReason=%q, clientRequest=%v)", p.TypeName, p.OptionalAllowReason, enableThinking)
		opts = append(opts, aispec.WithEnableThinking(true))
	} else if forceDisableThinking {
		log.Infof("GetAIClientWithRawMessages: enable_thinking=false (force disabled) for type=%s (OptionalAllowReason=%q)", p.TypeName, p.OptionalAllowReason)
		opts = append(opts, aispec.WithEnableThinking(false))
	}

	if onToolCall != nil {
		opts = append(opts, aispec.WithToolCallCallback(onToolCall))
	}
	if len(tools) > 0 {
		opts = append(opts, aispec.WithTools(tools))
	}
	if toolChoice != nil {
		opts = append(opts, aispec.WithToolChoice(toolChoice))
	}

	if target := strings.TrimSpace(p.DomainOrURL); target != "" {
		if utils.IsHttpOrHttpsUrl(target) {
			opts = append(opts, aispec.WithBaseURL(target))
		} else {
			opts = append(opts, aispec.WithDomain(target))
		}
	}
	client := ai.GetAI(p.TypeName, opts...)
	if utils.IsNil(client) || client == nil {
		return nil, errors.New("failed to get ai client, no such type: " + p.TypeName)
	}
	return client, nil
}

// GetAIClient gets the AI client
func (p *Provider) GetAIClient(enableThinking bool, onStream, onReasonStream func(reader io.Reader)) (aispec.AIClient, error) {
	return p.GetAIClientWithImages(nil, enableThinking, onStream, onReasonStream, nil)
}

// GetEmbeddingClient gets an embedding client for the provider
func (p *Provider) GetEmbeddingClient() (aispec.EmbeddingCaller, error) {
	log.Infof("GetEmbeddingClient: type: %s, domain: %s, key: %s, model: %s, no_https: %v",
		p.TypeName, p.DomainOrURL, utils.ShrinkString(p.APIKey, 8), p.ModelName, p.NoHTTPS)

	var opts []aispec.AIConfigOption
	opts = append(
		opts,
		aispec.WithNoHTTPS(p.NoHTTPS),
		aispec.WithAPIKey(p.APIKey),
		aispec.WithModel(p.ModelName),
		aispec.WithTimeout(30), // 30 seconds timeout for embedding
	)

	if target := strings.TrimSpace(p.DomainOrURL); target != "" {
		if utils.IsHttpOrHttpsUrl(target) {
			log.Infof("GetEmbeddingClient: Using BaseURL: %s", target)
			opts = append(opts, aispec.WithBaseURL(target))
		} else {
			log.Infof("GetEmbeddingClient: Using Domain: %s", target)
			opts = append(opts, aispec.WithDomain(target))
		}
	} else {
		log.Warnf("GetEmbeddingClient: No domain or URL specified, will use default")
	}

	log.Infof("GetEmbeddingClient: Creating OpenAI-compatible embedding client with %d options", len(opts))

	// Use the generic OpenAI-compatible embedding client
	// This works with most providers that support OpenAI's embedding API format
	client := embedding.NewOpenaiEmbeddingClient(opts...)
	if utils.IsNil(client) || client == nil {
		return nil, errors.New("failed to create embedding client")
	}

	log.Infof("GetEmbeddingClient: Successfully created embedding client for model: %s", p.ModelName)
	return client, nil
}

// GetDbProvider gets the associated database AiProvider object
// If no associated database object exists, try to query or create from database
func (p *Provider) GetDbProvider() (*schema.AiProvider, error) {
	// Use mutex to protect reading and setting of DbProvider
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// If already has associated database object, return directly
	if p.DbProvider != nil {
		return p.DbProvider, nil
	}

	// Create a temporary AiProvider object for querying
	dbProvider := &schema.AiProvider{
		WrapperName:        p.WrapperName,
		ModelName:          p.ModelName,
		TypeName:           p.TypeName,
		DomainOrURL:        p.DomainOrURL,
		APIKey:             p.APIKey,
		NoHTTPS:            p.NoHTTPS,
		ActiveCacheControl: p.ActiveCacheControl,
	}

	// Get or create from database
	dbAiProvider, err := GetOrCreateAiProvider(dbProvider)
	if err != nil {
		return nil, err
	}

	// Save association
	p.DbProvider = dbAiProvider
	return dbAiProvider, nil
}

// UpdateDbProvider updates the statistics of the associated database AiProvider object
// success: whether the request was successful
// latencyMs: request latency (milliseconds)
func (p *Provider) UpdateDbProvider(success bool, latencyMs int64) error {
	// Get database object
	dbProvider, err := p.GetDbProvider()
	if err != nil {
		return err
	}

	// Use Provider's mutex to protect concurrent updates
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Update statistics
	dbProvider.TotalRequests++
	dbProvider.LastRequestTime = time.Now()
	dbProvider.LastRequestStatus = success
	dbProvider.LastLatency = latencyMs

	if success {
		dbProvider.SuccessCount++
	} else {
		dbProvider.FailureCount++
	}

	// Update health status
	// If the last request failed or latency exceeded 3000ms, mark as unhealthy
	dbProvider.IsHealthy = success && latencyMs < 3000
	dbProvider.HealthCheckTime = time.Now()

	// Save to database
	return UpdateAiProvider(dbProvider)
}
