package openapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type OpenAPIConfig struct {
	Domain      string
	FlowHandler func(flow *yakit.HTTPFlow)
	IsHttps     bool
}

func NewDefaultOpenAPIConfig() *OpenAPIConfig {
	return &OpenAPIConfig{
		Domain: "www.example.com",
		FlowHandler: func(flow *yakit.HTTPFlow) {
			log.Infof("openapi generator create: %v", flow.Url)
		},
		IsHttps: false,
	}
}

type Option func(config *OpenAPIConfig)

func WithHttps(b bool) Option {
	return func(config *OpenAPIConfig) {
		config.IsHttps = b
	}
}

func WithDomain(domain string) Option {
	return func(config *OpenAPIConfig) {
		config.Domain = domain
	}
}

func WithFlowHandler(handler func(flow *yakit.HTTPFlow)) Option {
	return func(config *OpenAPIConfig) {
		config.FlowHandler = handler
	}
}
