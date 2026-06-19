package openapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

type OpenAPIConfig struct {
	Domain      string
	FlowHandler func(flow *schema.HTTPFlow)
	IsHttps     bool
}

func NewDefaultOpenAPIConfig() *OpenAPIConfig {
	return &OpenAPIConfig{
		FlowHandler: func(flow *schema.HTTPFlow) {
			log.Infof("openapi generator create: %v", flow.Url)
		},
		IsHttps: false,
	}
}

type Option func(config *OpenAPIConfig)

// WithHttps 设置生成的请求流是否使用 https（导出名为 openapi.https）
// 参数:
//   - b: 为 true 时使用 https
//
// 返回值:
//   - OpenAPI 生成可选项
//
// Example:
// ```
// // 示意性示例，需提供真实 OpenAPI 文档
// err = openapi.GenerateHTTPFlows(doc, openapi.https(true))
// ```
func WithHttps(b bool) Option {
	return func(config *OpenAPIConfig) {
		config.IsHttps = b
	}
}

// WithDomain 设置生成请求流时使用的目标域名（导出名为 openapi.domain）
// 参数:
//   - domain: 目标域名
//
// 返回值:
//   - OpenAPI 生成可选项
//
// Example:
// ```
// // 示意性示例，需提供真实 OpenAPI 文档
// err = openapi.GenerateHTTPFlows(doc, openapi.domain("example.com"))
// ```
func WithDomain(domain string) Option {
	return func(config *OpenAPIConfig) {
		config.Domain = domain
	}
}

// WithFlowHandler 设置接收生成 HTTP 流的回调（导出名为 openapi.flowHandler）
// 参数:
//   - handler: 处理每个生成 HTTP 流的回调函数
//
// 返回值:
//   - OpenAPI 生成可选项
//
// Example:
// ```
// // 示意性示例，需提供真实 OpenAPI 文档
//
//	err = openapi.GenerateHTTPFlows(doc, openapi.flowHandler(func(flow) {
//	    println(flow.Url)
//	}))
//
// ```
func WithFlowHandler(handler func(flow *schema.HTTPFlow)) Option {
	return func(config *OpenAPIConfig) {
		config.FlowHandler = handler
	}
}
