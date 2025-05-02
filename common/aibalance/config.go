package aibalance

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"

	"gopkg.in/yaml.v3"
)

type ModelConfig struct {
	Name      string            `yaml:"name" json:"name"`
	Providers []*ConfigProvider `yaml:"providers" json:"providers"`
}

type KeyConfig struct {
	Key           string   `yaml:"key" json:"key"`
	AllowedModels []string `yaml:"allowed_models" json:"allowed_models"`
}

type Config struct {
	Keys             *omap.OrderedMap[string, *KeyConfig]                     `json:"keys"`
	KeyAllowedModels *omap.OrderedMap[string, *omap.OrderedMap[string, bool]] `json:"key_allowed_models"`
	Models           *omap.OrderedMap[string, *ModelConfig]                   `json:"models"`

	Entrypoints *Entrypoint
}

func NewConfig() *Config {
	return &Config{
		Keys:             omap.NewOrderedMap[string, *KeyConfig](make(map[string]*KeyConfig)),
		KeyAllowedModels: omap.NewOrderedMap[string, *omap.OrderedMap[string, bool]](make(map[string]*omap.OrderedMap[string, bool])),
		Models:           omap.NewOrderedMap[string, *ModelConfig](make(map[string]*ModelConfig)),
		Entrypoints:      NewEntrypoint(),
	}
}

type YamlConfig struct {
	Keys   []KeyConfig   `yaml:"keys"`
	Models []ModelConfig `yaml:"models"`
}

func (c *YamlConfig) ToServerConfig() (*ServerConfig, error) {
	config := NewServerConfig()
	if config == nil {
		return nil, fmt.Errorf("failed to create server config")
	}

	fmt.Printf("YamlConfig.ToServerConfig: Processing config with %d keys and %d models\n", len(c.Keys), len(c.Models))

	// 处理 Keys
	for i, keyConfig := range c.Keys {
		fmt.Printf("YamlConfig.ToServerConfig: Processing key %d: %s with %d allowed models\n", i, keyConfig.Key, len(keyConfig.AllowedModels))
		key := &Key{
			Key:           keyConfig.Key,
			AllowedModels: make(map[string]bool),
		}
		for _, model := range keyConfig.AllowedModels {
			key.AllowedModels[model] = true
		}
		config.Keys.keys[keyConfig.Key] = key
		config.KeyAllowedModels.allowedModels[keyConfig.Key] = key.AllowedModels
	}

	// 处理 Models
	for i, model := range c.Models {
		fmt.Printf("YamlConfig.ToServerConfig: Processing model %d: %s with %d providers\n", i, model.Name, len(model.Providers))
		providers := make([]*Provider, 0)

		for j, configProvider := range model.Providers {
			if configProvider == nil {
				fmt.Printf("YamlConfig.ToServerConfig: Provider %d is nil, skipping\n", j)
				continue
			}

			fmt.Printf("YamlConfig.ToServerConfig: Processing provider %d: type=%s, domain=%s\n", j, configProvider.TypeName, configProvider.DomainOrURL)
			newProviders := configProvider.ToProviders()
			if newProviders == nil || len(newProviders) == 0 {
				fmt.Printf("YamlConfig.ToServerConfig: No providers returned from ToProviders for %s\n", configProvider.TypeName)
				continue
			}

			fmt.Printf("YamlConfig.ToServerConfig: Provider %d returned %d new providers\n", j, len(newProviders))
			for k, provider := range newProviders {
				if provider == nil {
					fmt.Printf("YamlConfig.ToServerConfig: New provider %d is nil, skipping\n", k)
					continue
				}

				// 设置模型名称（如果未设置）
				if provider.ModelName == "" {
					fmt.Printf("YamlConfig.ToServerConfig: Setting model name for provider %d.%d to %s\n", j, k, model.Name)
					provider.ModelName = model.Name
				}

				// 创建简单的内存中的 Provider
				provider = &Provider{
					ModelName:   provider.ModelName,
					TypeName:    provider.TypeName,
					DomainOrURL: provider.DomainOrURL,
					APIKey:      provider.APIKey,
					NoHTTPS:     provider.NoHTTPS,
				}
				providers = append(providers, provider)
				fmt.Printf("YamlConfig.ToServerConfig: Added provider to list: type=%s, model=%s\n", provider.TypeName, provider.ModelName)

				// 尝试保存 Provider 到数据库，但不影响主流程
				// 如果数据库操作失败，我们仍然可以使用内存中的配置
				dbProvider := &schema.AiProvider{
					WrapperName: model.Name,         // 设置外层名称(展示给用户的名称)
					ModelName:   provider.ModelName, // 保持内部实际使用的模型名称
					TypeName:    provider.TypeName,
					DomainOrURL: provider.DomainOrURL,
					APIKey:      provider.APIKey,
					NoHTTPS:     provider.NoHTTPS,
				}

				// 直接使用 GetOrCreateAiProvider 合并创建和更新操作
				_, _ = GetOrCreateAiProvider(dbProvider) // 忽略操作错误
			}
		}

		if len(providers) > 0 {
			fmt.Printf("YamlConfig.ToServerConfig: Setting %d providers for model %s\n", len(providers), model.Name)
			config.Models.models[model.Name] = providers
			config.Entrypoints.providers[model.Name] = providers
		} else {
			fmt.Printf("YamlConfig.ToServerConfig: No providers found for model %s\n", model.Name)
		}
	}

	return config, nil
}

func (c *YamlConfig) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, c)
}
