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

func (c *YamlConfig) ToConfig() *Config {
	cfg := NewConfig()

	log.Infof("开始转换配置，共有 %d 个模型和 %d 个密钥", len(c.Models), len(c.Keys))

	for _, model := range c.Models {
		log.Infof("添加模型: %s, 提供者数量: %d", model.Name, len(model.Providers))
		cfg.Models.Set(model.Name, &model)

		// 初始化 Entrypoints
		for _, provider := range model.Providers {
			cfg.Entrypoints.AddProvider(model.Name, provider)
		}
	}

	for _, key := range c.Keys {
		log.Infof("添加密钥: %s, 允许的模型: %v", key.Key, key.AllowedModels)
		cfg.Keys.Set(key.Key, &key)
		allowedModels := omap.NewOrderedMap[string, bool](make(map[string]bool))
		for _, model := range key.AllowedModels {
			log.Infof("密钥 %s 允许访问模型: %s", key.Key, model)
			allowedModels.Set(model, true)
		}
		cfg.KeyAllowedModels.Set(key.Key, allowedModels)
	}

	return cfg
}
