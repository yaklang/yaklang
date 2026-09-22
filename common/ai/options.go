package ai

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// WithExtraHeader 设置自定义请求头，支持 Yak map 和 json.loads 返回的有序映射。
// 请求头的值必须是字符串，空映射或 nil 不添加请求头。
//
// Example:
// ```
// option = ai.extraHeader({"X-Benchmark": "test-run"})
// ```
func WithExtraHeader(headers any) aispec.AIConfigOption {
	if utils.IsNil(headers) {
		return aispec.WithExtraHeader(nil)
	}
	if ordered, ok := headers.(interface{ ToStringMap() map[string]any }); ok {
		headers = ordered.ToStringMap()
	}
	values, err := utils.InterfaceToMapInterfaceE(headers)
	if err != nil {
		panic("ai.extraHeader expects a map of header names to string values")
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		text, ok := value.(string)
		if !ok {
			panic("ai.extraHeader expects string header values")
		}
		normalized[key] = text
	}
	return aispec.WithExtraHeader(normalized)
}
