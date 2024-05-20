package openapi

import "github.com/yaklang/yaklang/common/openapi/openapiyaml"

var Exports = map[string]any{
	"GenerateHTTPFlows":     GenerateHTTPFlows,
	"ExtractOpenAPI3Scheme": ExtractOpenAPI3Scheme,
	"ConvertJsonToYaml":     openapiyaml.JSONToYAML,
	"ConvertYamlToJson":     openapiyaml.YAMLToJSON,
	"https":                 WithHttps,
	"flowHandler":           WithFlowHandler,
	"domain":                WithDomain,
}
