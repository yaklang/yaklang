package aicommon

import "strings"

const (
	ExtraReActConfigKeyModelContextLevel = "ModelContextLevel"

	ModelContextLevelStandard = "standard"
	ModelContextLevelCompact  = "compact"

	extraReActConfigStoragePrefix = "extra_react_config."
)

func normalizeExtraReActConfigKey(key string) string {
	return strings.TrimSpace(key)
}

func extraReActConfigStorageKey(key string) string {
	return extraReActConfigStoragePrefix + strings.ToLower(normalizeExtraReActConfigKey(key))
}

func NormalizeModelContextLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "standard", "default", "full", "normal", "std", "标准":
		return ModelContextLevelStandard
	case "compact", "lite", "slim", "minimal", "concise", "精简", "简洁":
		return ModelContextLevelCompact
	default:
		return ModelContextLevelStandard
	}
}

func ResolveExtraReActConfigValue(cfg KeyValueConfigIf, key string) string {
	if cfg == nil {
		return ""
	}

	normalizedKey := normalizeExtraReActConfigKey(key)
	if normalizedKey == "" {
		return ""
	}

	if value := cfg.GetConfigString(normalizedKey); value != "" {
		return value
	}
	return cfg.GetConfigString(extraReActConfigStorageKey(normalizedKey))
}

func ResolveModelContextLevel(cfg KeyValueConfigIf) string {
	return NormalizeModelContextLevel(ResolveExtraReActConfigValue(cfg, ExtraReActConfigKeyModelContextLevel))
}

func WithExtraReActConfigMap(extra map[string]string) ConfigOption {
	return func(c *Config) error {
		if len(extra) == 0 {
			return nil
		}

		if c.ExtraReActConfig == nil {
			c.ExtraReActConfig = make(map[string]string)
		}

		for rawKey, rawValue := range extra {
			key := normalizeExtraReActConfigKey(rawKey)
			if key == "" {
				continue
			}

			value := strings.TrimSpace(rawValue)
			if strings.EqualFold(key, ExtraReActConfigKeyModelContextLevel) {
				value = NormalizeModelContextLevel(value)
				c.SetConfig(ExtraReActConfigKeyModelContextLevel, value)
			}

			c.ExtraReActConfig[key] = value
			c.SetConfig(extraReActConfigStorageKey(key), value)
		}
		return nil
	}
}

func WithModelContextLevel(level string) ConfigOption {
	return WithExtraReActConfigMap(map[string]string{
		ExtraReActConfigKeyModelContextLevel: level,
	})
}
