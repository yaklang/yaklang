package yakgrpc

import (
	"mime"
	"path"
	"strings"
	"sync"

	"github.com/gobwas/glob"
)

// isChunkStaticJSRequest 用于识别“chunk/static JS”这类通常带来高流量/高开销的静态资源请求（仅根据路径特征）。
//
// 说明：
// - 这里故意不依赖 MITMFilterData（避免用户保存过滤器后无法吃到默认策略更新）。
// - 规则尽量保守：只在扩展名为 .js 时才判断 chunk/static 特征，降低误判。
func isChunkStaticJSRequest(urlPath string) bool {
	// urlPath 仅使用 path（不含 query），调用方应传入类似 url.URL.EscapedPath()/Path
	p := strings.ToLower(strings.TrimSpace(urlPath))
	if p == "" || p[0] != '/' {
		// 保守：不是标准 path 的直接不认为是静态资源
		return false
	}

	// 必须是 .js 才考虑
	if path.Ext(p) != ".js" {
		return false
	}

	for _, g := range chunkStaticJSGlobs() {
		if g.Match(p) {
			return true
		}
	}
	return false
}

// isJavaScriptMIME 判断 Content-Type 是否为 JavaScript。
func isJavaScriptMIME(contentType string) bool {
	ct := strings.TrimSpace(strings.ToLower(contentType))
	if ct == "" {
		return false
	}
	parsed, _, err := mime.ParseMediaType(ct)
	if err == nil && parsed != "" {
		ct = parsed
	}

	switch ct {
	case "application/javascript",
		"application/x-javascript",
		"text/javascript",
		"application/ecmascript",
		"text/ecmascript":
		return true
	default:
		return strings.HasSuffix(ct, "+javascript")
	}
}

var (
	chunkStaticJSGlobsOnce sync.Once
	chunkStaticJSGlobList  []glob.Glob
)

func chunkStaticJSGlobs() []glob.Glob {
	chunkStaticJSGlobsOnce.Do(func() {
		patterns := []string{
			// 通用静态目录
			"/static/*.js",
			"/static/**/*.js",
			"/assets/*.js",
			"/assets/**/*.js",
			"/_next/static/**/*.js",

			// 常见文件命名（兜底）
			"/*.chunk.js",
			"/**/*.chunk.js",
		}

		chunkStaticJSGlobList = make([]glob.Glob, 0, len(patterns))
		for _, pat := range patterns {
			g, err := glob.Compile(strings.ToLower(pat))
			if err != nil {
				continue
			}
			chunkStaticJSGlobList = append(chunkStaticJSGlobList, g)
		}
	})
	return chunkStaticJSGlobList
}
