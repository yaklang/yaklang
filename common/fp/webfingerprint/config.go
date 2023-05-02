package webfingerprint

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

type Config struct {
	// 使用哪些规则来进行 Web 指纹探测？
	Rules []*WebRule

	// 主动模式
	ActiveMode bool

	// 强制所有规则进行匹配
	ForceAllRuleMatching bool

	// 在需要主动发送 Probe 的规则探测设置中
	//    我们需要为每一个 Probe 设置 TimeoutSeconds
	ProbeTimeout time.Duration

	// 指纹大小默认 20480
	FingerprintDataSize int

	// Proxies
	Proxies []string
}

func (c *Config) init() {
	c.ActiveMode = true
	c.ForceAllRuleMatching = true
	c.ProbeTimeout = 5 * time.Second
	c.FingerprintDataSize = 20480
}

func NewWebFingerprintConfig(options ...ConfigOption) *Config {
	config := &Config{}

	config.init()

	for _, option := range options {
		option(config)
	}

	return config
}

type ConfigOption func(config *Config)

func WithWebFingerprintRules(rules []*WebRule) ConfigOption {
	return func(config *Config) {
		config.Rules = rules
	}
}

func WithWebProxy(proxy ...string) ConfigOption {
	return func(config *Config) {
		config.Proxies = utils2.StringArrayFilterEmpty(proxy)
	}
}

func WithActiveMode(b bool) ConfigOption {
	return func(config *Config) {
		config.ActiveMode = b
	}
}

func WithForceAllRuleMatching(b bool) ConfigOption {
	return func(config *Config) {
		config.ForceAllRuleMatching = b
	}
}

func WithProbeTimeout(timeout time.Duration) ConfigOption {
	return func(config *Config) {
		config.ProbeTimeout = timeout
	}
}

func FileOrDirToWebRules(dir string) []*WebRule {
	if dir == "" {
		return nil
	}

	log.Infof("loading user web-fingerprint path: %s", dir)

	pathInfo, err := os.Stat(dir)
	if err != nil {
		log.Errorf("open path[%s] failed: %s", dir, err)
		return nil
	}

	if !pathInfo.IsDir() {
		raw, err := ioutil.ReadFile(dir)
		if err != nil {
			log.Error(err)
			return nil
		}
		rules, err := ParseWebFingerprintRules(raw)
		if err != nil {
			log.Error(err)
		}
		return rules
	}

	var rules []*WebRule
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && err == nil {
			return nil
		}

		fileName := filepath.Join(dir, info.Name())
		log.Infof("loading: %s", fileName)
		raw, err := ioutil.ReadFile(fileName)
		if err != nil {
			log.Error(err)
			return nil
		}
		r, err := ParseWebFingerprintRules(raw)
		if err != nil {
			log.Error(err)
		}
		rules = append(rules, r...)
		return nil
	})
	if err != nil {
		log.Error(err)
	}
	return rules
}
