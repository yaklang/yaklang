package hybridscan

import (
	"github.com/ReneKroon/ttlcache"
	"github.com/pkg/errors"
	"yaklang/common/fp"
	"yaklang/common/synscan"
	"time"
)

type Config struct {
	DisableFingerprintMatch        bool
	FingerprintMatcherConfig       *fp.Config
	FingerprintMatchQueueBuffer    int
	OpenPortTTLCache               *ttlcache.Cache
	FingerprintMatchResultTTLCache *ttlcache.Cache
	SynScanConfig                  *synscan.Config
}

type ConfigOption func(config *Config)

func NewConfig(options ...ConfigOption) *Config {
	c1 := ttlcache.NewCache()
	c1.SetTTL(1 * time.Hour)

	c2 := ttlcache.NewCache()
	c2.SetTTL(1 * time.Hour)

	config := &Config{
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

func NewDefaultConfigWithSynScanConfig(synScanConfig *synscan.Config, options ...ConfigOption) (*Config, error) {
	options = append([]ConfigOption{
		WithSynScanConfig(synScanConfig),
	}, options...)
	config := NewConfig(options...)
	return config, nil
}

func NewDefaultConfig(options ...ConfigOption) (*Config, error) {
	synScanConfig, err := synscan.NewDefaultConfig()
	if err != nil {
		return nil, errors.Errorf("create synscan config failed: %s", err)
	}

	return NewDefaultConfigWithSynScanConfig(synScanConfig, options...)
}

func WithFingerprintMatcherConfig(c *fp.Config) ConfigOption {
	return func(config *Config) {
		config.FingerprintMatcherConfig = c
	}
}

func WithFingerprintMatcherConfigOptions(options ...fp.ConfigOption) ConfigOption {
	return func(config *Config) {
		config.FingerprintMatcherConfig = fp.NewConfig(options...)
	}
}

func WithFingerprintMatchQueueBufferSize(size int) ConfigOption {
	return func(config *Config) {
		config.FingerprintMatchQueueBuffer = size
	}
}

func WithOpenPortTTLCache(ttl time.Duration) ConfigOption {
	return func(config *Config) {
		config.OpenPortTTLCache.Close()
		config.OpenPortTTLCache = ttlcache.NewCache()
		config.OpenPortTTLCache.SetTTL(ttl)
	}
}

func WithFingerprintMatchResultTTLCache(ttl time.Duration) ConfigOption {
	return func(config *Config) {
		config.FingerprintMatchResultTTLCache.Close()
		config.FingerprintMatchResultTTLCache = ttlcache.NewCache()
		config.FingerprintMatchResultTTLCache.SetTTL(ttl)
	}
}

func WithDisableFingerprintMatch(t bool) ConfigOption {
	return func(config *Config) {
		config.DisableFingerprintMatch = t
	}
}

func WithSynScanConfig(c *synscan.Config) ConfigOption {
	return func(config *Config) {
		config.SynScanConfig = c
	}
}
