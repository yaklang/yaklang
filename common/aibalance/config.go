package aibalance

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type ModelConfig struct {
	Name      string      `yaml:"name" json:"name"`
	Providers []*Provider `yaml:"providers" json:"providers"`
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

func (c *YamlConfig) ToServerConfig() *ServerConfig {
	cfg := NewServerConfig()

	log.Infof("Starting configuration conversion, total models: %d, total keys: %d", len(c.Models), len(c.Keys))

	for _, model := range c.Models {
		log.Infof("Adding model: %s, provider count: %d", model.Name, len(model.Providers))
		cfg.Models.models[model.Name] = model.Providers[0]

		// Initialize Entrypoints
		for _, provider := range model.Providers {
			cfg.Entrypoints.providers[model.Name] = provider
		}
	}

	for _, key := range c.Keys {
		log.Infof("Adding key: %s, allowed models: %v", key.Key, key.AllowedModels)
		cfg.Keys.keys[key.Key] = &Key{
			Key:           key.Key,
			AllowedModels: make(map[string]bool),
		}
		allowedModels := make(map[string]bool)
		for _, model := range key.AllowedModels {
			log.Infof("Key %s is allowed to access model: %s", key.Key, model)
			allowedModels[model] = true
		}
		cfg.KeyAllowedModels.allowedModels[key.Key] = allowedModels
	}

	return cfg
}
