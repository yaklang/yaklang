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

func (p *Provider) GetAIClientWithImages(imageContents []*aispec.ChatContent, onStream, onReasonStream func(reader io.Reader)) (aispec.AIClient, error) {
	log.Infof("GetAIClient: type: %s, domain: %s, key: %s, model: %s, no_https: %v", p.TypeName, p.DomainOrURL, utils.ShrinkString(p.APIKey, 8), p.ModelName, p.NoHTTPS)

	var images []any
	for _, content := range imageContents {
		if content.Type == "image_url" {
			images = append(images, content)
		}
	}
	var opts []aispec.AIConfigOption
	opts = append(
		opts,
		aispec.WithChatImageContent(images...),
		aispec.WithTimeout(10),
		aispec.WithNoHTTPS(p.NoHTTPS),
		aispec.WithAPIKey(p.APIKey),
		aispec.WithModel(p.ModelName),
		aispec.WithStreamHandler(func(reader io.Reader) {
			go func() {
				if onStream != nil {
					onStream(reader)
				} else {
					io.Copy(os.Stdout, reader)
				}
			}()
		}),
		aispec.WithReasonStreamHandler(func(reader io.Reader) {
			go func() {
				if onReasonStream != nil {
					onReasonStream(reader)
				} else {
					io.Copy(os.Stdout, reader)
				}
			}()
		}),
	)

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
func (p *Provider) GetAIClient(onStream, onReasonStream func(reader io.Reader)) (aispec.AIClient, error) {
	return p.GetAIClientWithImages(nil, onStream, onReasonStream)
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
			opts = append(opts, aispec.WithBaseURL(target))
		} else {
			opts = append(opts, aispec.WithDomain(target))
		}
	}

	// Use the generic OpenAI-compatible embedding client
	// This works with most providers that support OpenAI's embedding API format
	client := embedding.NewOpenaiEmbeddingClient(opts...)
	if utils.IsNil(client) || client == nil {
		return nil, errors.New("failed to create embedding client")
	}
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
		WrapperName: p.WrapperName,
		ModelName:   p.ModelName,
		TypeName:    p.TypeName,
		DomainOrURL: p.DomainOrURL,
		APIKey:      p.APIKey,
		NoHTTPS:     p.NoHTTPS,
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
