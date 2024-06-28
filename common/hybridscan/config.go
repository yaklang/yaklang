package hybridscan

import (
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
)

type HyperScanConfig struct {
	DisableFingerprintMatch        bool
	FingerprintMatcherConfig       *fp.Config
	FingerprintMatchQueueBuffer    int
	OpenPortTTLCache               *utils.Cache[int]
	FingerprintMatchResultTTLCache *utils.Cache[*fp.MatchResult]
	SynScanConfig                  *synscan.SynConfig
}

type HyperConfigOption func(config *HyperScanConfig)

func NewConfig(options ...HyperConfigOption) *HyperScanConfig {
	c1 := utils.NewTTLCache[int](1 * time.Hour)

	c2 := utils.NewTTLCache[*fp.MatchResult](1 * time.Hour)

	config := &HyperScanConfig{
		FingerprintMatcherConfig:       fp.NewConfig(),
		FingerprintMatchQueueBuffer:    100000,
		OpenPortTTLCache:               c1,
		FingerprintMatchResultTTLCache: c2,
	}

	for _, p := range options {
		p(config)
	}

	return config
}

func NewDefaultConfigWithSynScanConfig(synScanConfig *synscan.SynConfig, options ...HyperConfigOption) (*HyperScanConfig, error) {
	options = append(options, WithSynScanConfig(synScanConfig))
	config := NewConfig(options...)
	return config, nil
}

func NewDefaultConfig(options ...HyperConfigOption) (*HyperScanConfig, error) {
	synScanConfig, err := synscan.NewDefaultConfig()
	if err != nil {
		return nil, errors.Errorf("create synscan config failed: %s", err)
	}

	return NewDefaultConfigWithSynScanConfig(synScanConfig, options...)
}

func WithFingerprintMatcherConfig(c *fp.Config) HyperConfigOption {
	return func(config *HyperScanConfig) {
		config.FingerprintMatcherConfig = c
	}
}

func WithFingerprintMatcherConfigOptions(options ...fp.ConfigOption) HyperConfigOption {
	return func(config *HyperScanConfig) {
		config.FingerprintMatcherConfig = fp.NewConfig(options...)
	}
}

func WithFingerprintMatchQueueBufferSize(size int) HyperConfigOption {
	return func(config *HyperScanConfig) {
		config.FingerprintMatchQueueBuffer = size
	}
}

func WithOpenPortTTLCache(ttl time.Duration) HyperConfigOption {
	return func(config *HyperScanConfig) {
		config.OpenPortTTLCache.Close()
		config.OpenPortTTLCache = utils.NewTTLCache[int](ttl)
	}
}

func WithFingerprintMatchResultTTLCache(ttl time.Duration) HyperConfigOption {
	return func(config *HyperScanConfig) {
		config.FingerprintMatchResultTTLCache.Close()
		config.FingerprintMatchResultTTLCache = utils.NewTTLCache[*fp.MatchResult](ttl)
	}
}

func WithDisableFingerprintMatch(t bool) HyperConfigOption {
	return func(config *HyperScanConfig) {
		config.DisableFingerprintMatch = t
	}
}

func WithSynScanConfig(c *synscan.SynConfig) HyperConfigOption {
	return func(config *HyperScanConfig) {
		config.SynScanConfig = c
	}
}
