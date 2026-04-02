package aicommon

import (
	"errors"
	"strings"
)

const (
	ExtraReActConfigKeyModelContextLevel = "ModelContextLevel"

	ModelContextLevelStandard = "standard"
	ModelContextLevelCompact  = "compact"

	extraReActConfigStoragePrefix = "extra_react_config."

	standardPromptTimelineMaxBytes = 50 * 1024
	compactPromptTimelineMaxBytes  = 12 * 1024

	standardPromptToolCount = 15
	compactPromptToolCount  = 5

	standardPromptSkillCount = 12
	compactPromptSkillCount  = 5
)

type ModelContextProfile struct {
	Level string

	PromptTimelineMaxBytes int
	PromptToolCount        int
	PromptSkillCount       int
}

var ErrPromptFallbackNoMoreProfiles = errors.New("no more prompt compression profiles")

var modelContextProfiles = map[string]ModelContextProfile{
	ModelContextLevelStandard: {
		Level:                  ModelContextLevelStandard,
		PromptTimelineMaxBytes: standardPromptTimelineMaxBytes,
		PromptToolCount:        standardPromptToolCount,
		PromptSkillCount:       standardPromptSkillCount,
	},
	ModelContextLevelCompact: {
		Level:                  ModelContextLevelCompact,
		PromptTimelineMaxBytes: compactPromptTimelineMaxBytes,
		PromptToolCount:        compactPromptToolCount,
		PromptSkillCount:       compactPromptSkillCount,
	},
}

func sameModelContextProfile(a, b ModelContextProfile) bool {
	return a.PromptTimelineMaxBytes == b.PromptTimelineMaxBytes &&
		a.PromptToolCount == b.PromptToolCount &&
		a.PromptSkillCount == b.PromptSkillCount
}

func minPositiveInt(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func appendGradientProfile(base ModelContextProfile, target ModelContextProfile, profiles *[]ModelContextProfile) {
	candidate := ModelContextProfile{
		Level:                  target.Level,
		PromptTimelineMaxBytes: minPositiveInt(base.PromptTimelineMaxBytes, target.PromptTimelineMaxBytes),
		PromptToolCount:        minPositiveInt(base.PromptToolCount, target.PromptToolCount),
		PromptSkillCount:       minPositiveInt(base.PromptSkillCount, target.PromptSkillCount),
	}
	if sameModelContextProfile(candidate, base) {
		return
	}
	for _, existing := range *profiles {
		if sameModelContextProfile(existing, candidate) {
			return
		}
	}
	*profiles = append(*profiles, candidate)
}

func BuildGradientModelContextProfiles(base ModelContextProfile) []ModelContextProfile {
	var profiles []ModelContextProfile

	appendGradientProfile(base, ModelContextProfile{
		Level:                  "gradient-medium",
		PromptTimelineMaxBytes: 24 * 1024,
		PromptToolCount:        8,
		PromptSkillCount:       6,
	}, &profiles)
	appendGradientProfile(base, GetModelContextProfile(ModelContextLevelCompact), &profiles)
	appendGradientProfile(base, ModelContextProfile{
		Level:                  "gradient-lite",
		PromptTimelineMaxBytes: 8 * 1024,
		PromptToolCount:        3,
		PromptSkillCount:       3,
	}, &profiles)
	appendGradientProfile(base, ModelContextProfile{
		Level:                  "gradient-minimal",
		PromptTimelineMaxBytes: 4 * 1024,
		PromptToolCount:        1,
		PromptSkillCount:       1,
	}, &profiles)

	return profiles
}

func SelectGradientModelContextProfileByLevel(profiles []ModelContextProfile, level int) (ModelContextProfile, bool) {
	if level < 0 || level >= len(profiles) {
		return ModelContextProfile{}, false
	}
	return profiles[level], true
}

func SelectGradientModelContextProfile(profiles []ModelContextProfile, expectedContextSize int, currentContextSize int) (ModelContextProfile, bool) {
	if len(profiles) == 0 || expectedContextSize <= 0 || currentContextSize <= expectedContextSize {
		return ModelContextProfile{}, false
	}

	ratio := float64(currentContextSize) / float64(expectedContextSize)
	idx := len(profiles) - 1
	switch {
	case ratio > 3:
		idx = 0
	case ratio > 2:
		if len(profiles) > 1 {
			idx = 1
		}
	case ratio > 1.3:
		if len(profiles) > 2 {
			idx = 2
		}
	}

	if idx < 0 || idx >= len(profiles) {
		return ModelContextProfile{}, false
	}
	return profiles[idx], true
}

func NewGradientPromptFallback(base ModelContextProfile, render func(profile ModelContextProfile) (string, error)) PromptFallback {
	if render == nil {
		return nil
	}

	profiles := BuildGradientModelContextProfiles(base)
	if len(profiles) == 0 {
		return nil
	}

	return func(expectedContextSize int, currentContextSize int, compressionLevel int) (string, error) {
		if expectedContextSize <= 0 || currentContextSize <= expectedContextSize {
			return "", nil
		}

		profile, ok := SelectGradientModelContextProfileByLevel(profiles, compressionLevel)
		if !ok {
			return "", ErrPromptFallbackNoMoreProfiles
		}
		return render(profile)
	}
}

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

func GetModelContextProfile(level string) ModelContextProfile {
	normalizedLevel := NormalizeModelContextLevel(level)
	if profile, ok := modelContextProfiles[normalizedLevel]; ok {
		return profile
	}
	return modelContextProfiles[ModelContextLevelStandard]
}

func ResolveModelContextProfile(cfg KeyValueConfigIf) ModelContextProfile {
	return GetModelContextProfile(ResolveModelContextLevel(cfg))
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
