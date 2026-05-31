package airaghttp

import (
	_ "embed"
	"strings"
)

// frontendHTML 内置的只读搜索前端页面 (Codex 风格输入框 + SSE 流式渲染)
// 关键词: go:embed frontend, read-only search UI, SSE
//
//go:embed web/index.html
var frontendHTML string

// 页面中用于注入实际路由前缀的占位符
const frontendPrefixPlaceholder = "__RAG_ROUTE_PREFIX__"

// renderFrontendHTML 将占位符替换为实际路由前缀后返回完整 HTML
func renderFrontendHTML(routePrefix string) string {
	if routePrefix == "" {
		routePrefix = "/api/rag-server"
	}
	return strings.ReplaceAll(frontendHTML, frontendPrefixPlaceholder, routePrefix)
}
