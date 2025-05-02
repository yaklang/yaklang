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
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// ConfigProvider 用于配置文件的 provider 结构体
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

// Provider 用于实际 API 调用的 provider 结构体
type Provider struct {
	ModelName   string `json:"model_name"`
	TypeName    string `json:"type_name"`
	DomainOrURL string `json:"domain_or_url"`
	APIKey      string `json:"api_key"`
	NoHTTPS     bool   `json:"no_https"`
	// works for qwen3
	OptionalAllowReason  string `json:"optional_allow_reason,omitempty"`
	OptionalReasonBudget int    `json:"optional_reason_budget,omitempty"`

	// 数据库中对应的 AiProvider 对象
	DbProvider *schema.AiProvider `json:"-"` // json:"-" 表示此字段不会被序列化到 JSON

	// 保护并发更新的互斥锁
	mutex sync.Mutex `json:"-"`
}

// toProvider 将 ConfigProvider 转换为 Provider（私有方法）
func (cp *ConfigProvider) toProvider(apiKey string) *Provider {
	// 确保至少有 TypeName
	if cp.TypeName == "" {
		log.Errorf("Provider type name cannot be empty")
		return nil
	}

	// 确保 DomainOrURL 有效（对大多数提供者来说是必需的）
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
	}
}

// ToProviders 将 ConfigProvider 转换为多个 Provider
func (cp *ConfigProvider) ToProviders() []*Provider {
	providers := make([]*Provider, 0)

	// 验证必要字段
	if cp.TypeName == "" {
		log.Errorf("Provider type name cannot be empty")
		return nil
	}

	allKeys := cp.GetAllKeys()

	// 如果没有可用的 keys，只有在某些情况下才使用默认的 provider
	if len(allKeys) == 0 {
		// 某些类型的提供者可能不需要 API 密钥（例如本地模型或开源模型）
		if cp.TypeName == "ollama" {
			// 对于不需要 API 密钥的提供者，使用默认的 provider
			providers = append(providers, cp.toProvider(""))
			return providers
		}

		log.Warnf("No API keys available for provider type: %s", cp.TypeName)
		return nil // 没有可用的密钥，返回空
	}

	// 为每个 key 创建一个新的 provider
	for _, key := range allKeys {
		log.Infof("ToProviders: type: %v, model: %s, key: %s", cp.TypeName, cp.ModelName, utils.ShrinkString(key, 8))
		provider := cp.toProvider(key)
		if provider != nil {
			providers = append(providers, provider)
		}
	}

	return providers
}

// GetAllKeys 获取所有可用的 keys
func (cp *ConfigProvider) GetAllKeys() []string {
	var allKeys []string

	// 检查直接配置的 API 密钥
	if cp.APIKey != "" {
		allKeys = append(allKeys, cp.APIKey)
	}

	// 检查密钥列表
	if len(cp.Keys) > 0 {
		allKeys = append(allKeys, cp.Keys...)
	}

	// 检查 KeyFile（包含多个密钥的文件）
	if cp.KeyFile != "" {
		// 尝试读取文件
		data, err := os.ReadFile(cp.KeyFile)
		if err != nil {
			log.Errorf("Failed to read key file %s: %v", cp.KeyFile, err)
		} else {
			// 按行分割，每行一个密钥
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

// GetAIClient 获取 AI 客户端
func (p *Provider) GetAIClient(onStream, onReasonStream func(reader io.Reader)) (aispec.AIClient, error) {
	log.Infof("GetAIClient: type: %s, domain: %s, key: %s, model: %s, no_https: %v", p.TypeName, p.DomainOrURL, utils.ShrinkString(p.APIKey, 8), p.ModelName, p.NoHTTPS)
	client := ai.GetAI(
		p.TypeName,
		aispec.WithTimeout(10),
		aispec.WithNoHTTPS(p.NoHTTPS),
		aispec.WithAPIKey(p.APIKey),
		aispec.WithBaseURL(p.DomainOrURL),
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
	if utils.IsNil(client) || client == nil {
		return nil, errors.New("failed to get ai client, no such type: " + p.TypeName)
	}
	return client, nil
}

// GetDbProvider 获取关联的数据库 AiProvider 对象
// 如果没有关联的数据库对象，尝试从数据库查询或创建
func (p *Provider) GetDbProvider() (*schema.AiProvider, error) {
	// 使用互斥锁保护 DbProvider 的读取和设置
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 如果已经有关联的数据库对象，直接返回
	if p.DbProvider != nil {
		return p.DbProvider, nil
	}

	// 创建一个临时的 AiProvider 对象用于查询
	dbProvider := &schema.AiProvider{
		ModelName:   p.ModelName,
		TypeName:    p.TypeName,
		DomainOrURL: p.DomainOrURL,
		APIKey:      p.APIKey,
		NoHTTPS:     p.NoHTTPS,
	}

	// 从数据库获取或创建
	dbAiProvider, err := GetOrCreateAiProvider(dbProvider)
	if err != nil {
		return nil, err
	}

	// 保存关联
	p.DbProvider = dbAiProvider
	return dbAiProvider, nil
}

// UpdateDbProvider 更新关联的数据库 AiProvider 对象的统计信息
// success：请求是否成功
// latencyMs：请求延迟（毫秒）
func (p *Provider) UpdateDbProvider(success bool, latencyMs int64) error {
	// 获取数据库对象
	dbProvider, err := p.GetDbProvider()
	if err != nil {
		return err
	}

	// 使用 Provider 自身的互斥锁保护并发更新
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 更新统计信息
	dbProvider.TotalRequests++
	dbProvider.LastRequestTime = time.Now()
	dbProvider.LastRequestStatus = success
	dbProvider.LastLatency = latencyMs

	if success {
		dbProvider.SuccessCount++
	} else {
		dbProvider.FailureCount++
	}

	// 更新健康状态
	// 如果最后一次请求失败或延迟超过3000ms，则标记为不健康
	dbProvider.IsHealthy = success && latencyMs < 3000
	dbProvider.HealthCheckTime = time.Now()

	// 保存到数据库
	return UpdateAiProvider(dbProvider)
}
