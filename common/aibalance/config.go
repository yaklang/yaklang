package aibalance

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/log"
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
	Keys   []KeyConfig   `yaml:"keys" json:"keys"`
	Models []ModelConfig `yaml:"models" json:"models"`
}

// ToServerConfig converts YamlConfig to ServerConfig
func (c *YamlConfig) ToServerConfig() (*ServerConfig, error) {
	config := NewServerConfig()
	if config == nil {
		return nil, fmt.Errorf("failed to create server config")
	}

	// Record mapping relationship between API keys and allowed models
	log.Debugf("YamlConfig.ToServerConfig: Processing config with %d keys and %d models", len(c.Keys), len(c.Models))

	// Process API keys
	for i, keyConfig := range c.Keys {
		log.Debugf("YamlConfig.ToServerConfig: Processing key %d: %s with %d allowed models", i, keyConfig.Key, len(keyConfig.AllowedModels))

		// Set up API key
		key := &Key{
			Key:           keyConfig.Key,
			AllowedModels: make(map[string]bool),
		}
		for _, model := range keyConfig.AllowedModels {
			key.AllowedModels[model] = true
		}
		config.Keys.keys[keyConfig.Key] = key
		config.KeyAllowedModels.allowedModels[keyConfig.Key] = key.AllowedModels

		// Set models allowed for this key
		// Already set up above
	}

	// Process model configurations
	for i, model := range c.Models {
		log.Debugf("YamlConfig.ToServerConfig: Processing model %d: %s with %d providers", i, model.Name, len(model.Providers))

		// Get all providers for this model
		var providers []*Provider

		// Process all providers for this model
		for j, configProvider := range model.Providers {
			if configProvider == nil {
				log.Debugf("YamlConfig.ToServerConfig: Provider %d is nil, skipping", j)
				continue
			}

			log.Debugf("YamlConfig.ToServerConfig: Processing provider %d: type=%s, domain=%s", j, configProvider.TypeName, configProvider.DomainOrURL)
			newProviders := configProvider.ToProviders()
			if newProviders == nil || len(newProviders) == 0 {
				log.Debugf("YamlConfig.ToServerConfig: No providers returned from ToProviders for %s", configProvider.TypeName)
				continue
			}

			log.Debugf("YamlConfig.ToServerConfig: Provider %d returned %d new providers", j, len(newProviders))
			for k, provider := range newProviders {
				if provider == nil {
					log.Debugf("YamlConfig.ToServerConfig: New provider %d is nil, skipping", k)
					continue
				}

				// If provider doesn't specify model name, use current model's name
				if provider.ModelName == "" {
					log.Debugf("YamlConfig.ToServerConfig: Setting model name for provider %d.%d to %s", j, k, model.Name)
					provider.ModelName = model.Name
				}

				// 设置 provider 的 WrapperName 为当前模型的名称（外部展示名称）
				provider.WrapperName = model.Name
				log.Debugf("YamlConfig.ToServerConfig: Setting wrapper name for provider %d.%d to %s", j, k, model.Name)

				// Add to provider list
				providers = append(providers, provider)

				// Ensure both model and type are valid
				if provider.ModelName == "" || provider.TypeName == "" {
					continue
				}
				log.Debugf("YamlConfig.ToServerConfig: Added provider to list: type=%s, model=%s, wrapper=%s", provider.TypeName, provider.ModelName, provider.WrapperName)
			}
		}

		// Ensure provider list is not empty
		if len(providers) > 0 {
			// Add providers to Models (indexed by actual model name)
			config.Models.models[model.Name] = providers

			// Add providers to Entrypoints (indexed by display name)
			for _, provider := range providers {
				config.Entrypoints.Add(model.Name, []*Provider{provider})
			}

			log.Debugf("YamlConfig.ToServerConfig: Setting %d providers for model %s", len(providers), model.Name)
		} else {
			log.Debugf("YamlConfig.ToServerConfig: No providers found for model %s", model.Name)
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
