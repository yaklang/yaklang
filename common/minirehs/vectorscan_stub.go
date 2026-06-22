//go:build !minirehs_vectorscan

package minirehs

// 本文件在默认构建 (未启用 minirehs_vectorscan) 下生效: Vectorscan 后端不编入二进制,
// 保证默认产物零原生依赖、全平台可移植。请求该后端时 selectBackend 会优雅退化为引擎。

// newVectorscanBackend 在默认构建下返回 nil, 表示不可用 (调用方据此退化为引擎)。
func newVectorscanBackend() backendImpl { return nil }
