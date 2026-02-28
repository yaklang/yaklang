package aiconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v3"
)

type AIModelConfigEntry struct {
	Type        string            `json:"type" yaml:"type"`
	APIKey      string            `json:"api_key" yaml:"api_key"`
	Domain      string            `json:"domain" yaml:"domain"`
	Model       string            `json:"model" yaml:"model"`
	ExtraParams map[string]string `json:"extra_params,omitempty" yaml:"extra_params,omitempty"`
}

type TieredAIConfigFile struct {
	Enabled            bool                 `json:"enabled" yaml:"enabled"`
	RoutingPolicy      string               `json:"routing_policy" yaml:"routing_policy"`
	DisableFallback    bool                 `json:"disable_fallback" yaml:"disable_fallback"`
	IntelligentConfigs []AIModelConfigEntry `json:"intelligent_configs" yaml:"intelligent_configs"`
	LightweightConfigs []AIModelConfigEntry `json:"lightweight_configs" yaml:"lightweight_configs"`
	VisionConfigs      []AIModelConfigEntry `json:"vision_configs" yaml:"vision_configs"`
}

func ConfigEntryToThirdPartyConfig(entry AIModelConfigEntry) *ypb.ThirdPartyApplicationConfig {
	cfg := &ypb.ThirdPartyApplicationConfig{
		Type:   entry.Type,
		APIKey: entry.APIKey,
		Domain: entry.Domain,
	}
	if entry.Model != "" {
		cfg.ExtraParams = append(cfg.ExtraParams, &ypb.KVPair{Key: "model", Value: entry.Model})
	}
	for k, v := range entry.ExtraParams {
		if k == "model" {
			continue
		}
		cfg.ExtraParams = append(cfg.ExtraParams, &ypb.KVPair{Key: k, Value: v})
	}
	return cfg
}

func ConfigEntryToModelConfig(entry AIModelConfigEntry) *ypb.AIModelConfig {
	provider := &ypb.ThirdPartyApplicationConfig{
		Type:   entry.Type,
		APIKey: entry.APIKey,
		Domain: entry.Domain,
	}
	extras := make([]*ypb.KVPair, 0, len(entry.ExtraParams))
	for k, v := range entry.ExtraParams {
		if k == consts.ModelExtraParamKey {
			continue
		}
		extras = append(extras, &ypb.KVPair{Key: k, Value: v})
	}
	return &ypb.AIModelConfig{
		Provider:    provider,
		ModelName:   entry.Model,
		ExtraParams: extras,
	}
}

func ThirdPartyConfigToEntry(cfg *ypb.ThirdPartyApplicationConfig) AIModelConfigEntry {
	entry := AIModelConfigEntry{
		Type:   cfg.GetType(),
		APIKey: cfg.GetAPIKey(),
		Domain: cfg.GetDomain(),
	}
	for _, kv := range cfg.GetExtraParams() {
		if kv.GetKey() == "model" {
			entry.Model = kv.GetValue()
		}
	}
	extras := make(map[string]string)
	for _, kv := range cfg.GetExtraParams() {
		if kv.GetKey() != "model" {
			extras[kv.GetKey()] = kv.GetValue()
		}
	}
	if len(extras) > 0 {
		entry.ExtraParams = extras
	}
	return entry
}

func ConfigFileToTieredAIConfig(cfg *TieredAIConfigFile) *consts.TieredAIConfig {
	tiered := &consts.TieredAIConfig{
		Enabled:         cfg.Enabled,
		DisableFallback: cfg.DisableFallback,
	}
	switch cfg.RoutingPolicy {
	case "auto":
		tiered.RoutingPolicy = consts.PolicyAuto
	case "performance":
		tiered.RoutingPolicy = consts.PolicyPerformance
	case "cost":
		tiered.RoutingPolicy = consts.PolicyCost
	case "balance":
		tiered.RoutingPolicy = consts.PolicyBalance
	default:
		tiered.RoutingPolicy = consts.PolicyBalance
	}
	for _, e := range cfg.IntelligentConfigs {
		tiered.IntelligentConfigs = append(tiered.IntelligentConfigs, ConfigEntryToModelConfig(e))
	}
	for _, e := range cfg.LightweightConfigs {
		tiered.LightweightConfigs = append(tiered.LightweightConfigs, ConfigEntryToModelConfig(e))
	}
	for _, e := range cfg.VisionConfigs {
		tiered.VisionConfigs = append(tiered.VisionConfigs, ConfigEntryToModelConfig(e))
	}
	return tiered
}

func GetDefaultTieredAIConfigFile() *TieredAIConfigFile {
	return &TieredAIConfigFile{
		Enabled:         true,
		RoutingPolicy:   "balance",
		DisableFallback: false,
		IntelligentConfigs: []AIModelConfigEntry{
			{Type: "aibalance", APIKey: "free-user", Domain: "aibalance.yaklang.com", Model: "memfit-standard-free"},
		},
		LightweightConfigs: []AIModelConfigEntry{
			{Type: "aibalance", APIKey: "free-user", Domain: "aibalance.yaklang.com", Model: "memfit-light-free"},
		},
		VisionConfigs: []AIModelConfigEntry{
			{Type: "aibalance", APIKey: "free-user", Domain: "aibalance.yaklang.com", Model: "memfit-vision-free"},
		},
	}
}

func GetDefaultConfigDir() string {
	return filepath.Join(consts.GetDefaultYakitBaseDir(), "base")
}

func GetDefaultConfigPaths() []string {
	dir := GetDefaultConfigDir()
	return []string{
		filepath.Join(dir, "tiered-ai-config.yaml"),
		filepath.Join(dir, "tiered-ai-config.json"),
	}
}

func ResolveConfigFilePath(specified string) string {
	if specified != "" {
		return specified
	}
	for _, p := range GetDefaultConfigPaths() {
		if utils.GetFirstExistedFile(p) != "" {
			return p
		}
	}
	return filepath.Join(GetDefaultConfigDir(), "tiered-ai-config.yaml")
}

// SaveTieredAIConfigToDB persists the given TieredAIConfigFile into the database
// via the AI global config storage. This is the authoritative way to update
// tiered AI configuration -- all runtime reads should come from the database.
func SaveTieredAIConfigToDB(cfg *TieredAIConfigFile) error {
	aiConfig := &ypb.AIGlobalConfig{
		Enabled:         cfg.Enabled,
		RoutingPolicy:   cfg.RoutingPolicy,
		DisableFallback: cfg.DisableFallback,
	}

	for _, e := range cfg.IntelligentConfigs {
		aiConfig.IntelligentModels = append(aiConfig.IntelligentModels, ConfigEntryToModelConfig(e))
	}
	for _, e := range cfg.LightweightConfigs {
		aiConfig.LightweightModels = append(aiConfig.LightweightModels, ConfigEntryToModelConfig(e))
	}
	for _, e := range cfg.VisionConfigs {
		aiConfig.VisionModels = append(aiConfig.VisionModels, ConfigEntryToModelConfig(e))
	}

	if _, err := yakit.SetAIGlobalConfig(consts.GetGormProfileDatabase(), aiConfig); err != nil {
		return err
	}
	_ = yakit.ApplyAIGlobalConfig(consts.GetGormProfileDatabase(), aiConfig)
	log.Infof("tiered AI config saved to database: enabled=%v, policy=%s", cfg.Enabled, cfg.RoutingPolicy)
	return nil
}

func LoadTieredAIConfigFile(path string) (*TieredAIConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, utils.Errorf("failed to read config file %s: %v", path, err)
	}

	cfg := &TieredAIConfigFile{}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, utils.Errorf("failed to parse YAML config: %v", err)
		}
	case ".json":
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, utils.Errorf("failed to parse JSON config: %v", err)
		}
	default:
		if err := yaml.Unmarshal(data, cfg); err != nil {
			if err2 := json.Unmarshal(data, cfg); err2 != nil {
				return nil, utils.Errorf("failed to parse config (tried YAML and JSON): yaml=%v, json=%v", err, err2)
			}
		}
	}
	return cfg, nil
}
