package config

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
)

type PageConfig struct {
	urlStr string `default:"localhost"`

	proxyAddress  string `default:""`
	proxyUsername string `default:""`
	proxyPassword string `default:""`

	wsAddress string `default:""`
	exePath   string `default:""`

	ctx context.Context
}

func (pc *PageConfig) Url() string {
	return pc.urlStr
}

func (pc *PageConfig) Proxy() (string, string, string) {
	return pc.proxyAddress, pc.proxyUsername, pc.proxyPassword
}

func (pc *PageConfig) WsAddress() string {
	return pc.wsAddress
}

func (pc *PageConfig) ExePath() string {
	return pc.exePath
}

func (pageConfig *PageConfig) Context() context.Context {
	return pageConfig.ctx
}

type ConfigFunc func(config *PageConfig)

func WithUrlConfig(urlStr string) ConfigFunc {
	return func(config *PageConfig) {
		config.urlStr = urlStr
	}
}

func WithProxyConfig(proxyStrs ...string) ConfigFunc {
	if len(proxyStrs) == 1 {
		return func(config *PageConfig) {
			config.proxyAddress = proxyStrs[0]
		}
	} else if len(proxyStrs) == 3 {
		return func(config *PageConfig) {
			config.proxyAddress = proxyStrs[0]
			config.proxyUsername = proxyStrs[1]
			config.proxyPassword = proxyStrs[2]
		}
	}
	log.Errorf("proxy length error: %s", proxyStrs)
	return func(config *PageConfig) {}
}

func WithContext(context context.Context) ConfigFunc {
	return func(config *PageConfig) {
		config.ctx = context
	}
}

func WithWsAddress(wsAddress string) ConfigFunc {
	return func(config *PageConfig) {
		config.wsAddress = wsAddress
	}
}

func WithExePath(exePath string) ConfigFunc {
	return func(config *PageConfig) {
		config.exePath = exePath
	}
}
