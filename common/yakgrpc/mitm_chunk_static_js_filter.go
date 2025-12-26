package yakgrpc

import (
	"bytes"
	"mime"
	"path"
	"regexp"
	"strconv"
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

	return isBundledJavaScriptStrongPath(p)
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

// shouldFilterBundledJavaScript 用于过滤“编译/打包后的静态 JS”，尽量减少误伤重要的业务 JS。
//
// 具体判断标准（默认策略，尽量保守 + 性能优先）：
//
// 0) 必须先满足：
//   - 响应 Content-Type 判定为 JavaScript（isJavaScriptMIME）
//   - URL Path 是以 "/" 开头的标准路径，且扩展名为 ".js"
//
// 1) 若命中【强路径】（几乎确定是打包产物），直接过滤：
//   - Next.js / React 常见构建产物："/_next/static/**/*.js"
//   - Webpack 常见分包目录："/static/js/**/*.js"、"/static/chunks/**/*.js"
//   - 典型命名："/**/*.chunk.js"
//
// 2) 若不命中强路径：
//   - 若也不命中【弱路径】（"/static/**/*.js" 或 "/assets/**/*.js"），不过滤（避免误伤 API 返回 JS 等）
//
// 3) 命中弱路径时（static/assets 里可能混有手写 JS），需要额外“强信号”才过滤：
//   - 文件名带 hash（如 main.0b8e744a.js / index-7a0f5f5d.js / 0b8e-7a0f....js）
//     -> 直接过滤
//   - 或 Cache-Control 具备强缓存特征：
//   - 包含 "immutable"
//   - 或 max-age >= 86400（>= 1 天）
//     -> 直接过滤
//
// 4) 若弱路径下既没有 hash 也没有强缓存，则尝试用 body 的“打包特征”做最后判定（性能限制）：
//   - 仅扫描 body 前 64KB（避免大包扫描开销）
//   - 强特征（命中即可认为打包产物）：__webpack_require__ / webpackChunk
//   - 中等特征：regeneratorRuntime. / /*#__PURE__*/ / var _interopRequireDefault / Object.defineProperty + "__esModule"
//   - 弱特征："use strict";（单独使用容易误伤，仅作为弱加分）
//
// 5) 最终在弱路径下的过滤条件为：
//   - signatureScore >= 2  -> 过滤
//   - 或 (Content-Length >= 200KB 且 signatureScore >= 1) -> 过滤
func shouldFilterBundledJavaScript(urlPath string, contentType string, cacheControl string, contentLength int64, body []byte) bool {
	if !isJavaScriptMIME(contentType) {
		return false
	}

	p := strings.ToLower(strings.TrimSpace(urlPath))
	if p == "" || p[0] != '/' || path.Ext(p) != ".js" {
		return false
	}

	filename := path.Base(p)
	hasHash := hasHashedJSFilename(filename)
	cacheStrong := isCacheControlStrong(cacheControl)

	// 对不在 static/assets 的 JS 也提供“非常保守”的兜底：
	// - hash + 强缓存：基本可以认为是构建产物（常见于根路径的 framework-xxxx.js）
	// - 超大 + 强缓存 + min.js：多为第三方库（如 babel.min.js），通常对抓包分析意义不大
	if !isBundledJavaScriptWeakPath(p) && !isBundledJavaScriptStrongPath(p) {
		if hasHash && cacheStrong {
			return true
		}
		if cacheStrong && contentLength >= 512*1024 && strings.Contains(filename, ".min.") {
			return true
		}
		return false
	}

	// 强路径：框架/脚手架固定产物目录，基本都属于编译产物
	if isBundledJavaScriptStrongPath(p) {
		return true
	}

	// 弱路径：static/assets 目录可能包含手写 JS，需要额外信号
	// 只要 hash 或强缓存其一成立，就可以认为是编译产物
	if hasHash || cacheStrong {
		return true
	}

	sigScore := bundledJSSignatureScore(body)

	// 没有 hash/强缓存时，仅在 body 强特征或体积很大时过滤
	if sigScore >= 2 {
		return true
	}
	if contentLength >= 200*1024 && sigScore >= 1 {
		return true
	}

	return false
}

func bundledJSSignatureScore(body []byte) int {
	// 仅扫描前 64KB，足够命中 webpack 引导等特征，同时控制开销
	const maxScan = 64 * 1024
	if len(body) == 0 {
		return 0
	}
	if len(body) > maxScan {
		body = body[:maxScan]
	}

	// 强特征（命中基本可以认为是打包产物）
	if bytes.Contains(body, []byte("__webpack_require__")) ||
		bytes.Contains(body, []byte(`self["webpackChunk`)) ||
		bytes.Contains(body, []byte(`self['webpackChunk`)) {
		return 2
	}

	// 中等特征（配合体积/缓存头更可靠）
	score := 0
	if bytes.Contains(body, []byte("regeneratorRuntime.")) {
		score++
	}
	if bytes.Contains(body, []byte("/*#__PURE__*/")) {
		score++
	}
	if bytes.Contains(body, []byte("var _interopRequireDefault")) {
		score++
	}
	if bytes.Contains(body, []byte("Object.defineProperty")) && bytes.Contains(body, []byte(`"__esModule"`)) {
		score++
	}

	// 弱特征：手写 JS 也可能有 "use strict"，不单独作为依据
	if score == 0 && bytes.Contains(body, []byte(`"use strict";`)) {
		return 1
	}
	if score > 1 {
		return 2
	}
	return score
}

var (
	hashedJSFilenameOnce sync.Once
	hashedJSFilenameRe   []*regexp.Regexp
)

func hasHashedJSFilename(filename string) bool {
	filename = strings.ToLower(strings.TrimSpace(filename))
	if filename == "" {
		return false
	}

	hashedJSFilenameOnce.Do(func() {
		hashedJSFilenameRe = []*regexp.Regexp{
			// foo.[hash].js / foo-[hash].js / foo_hash.js
			regexp.MustCompile(`(?i)[._-][a-f0-9]{8,}\.js$`),
			// next.js 常见：[hash]-[hash].js / 多段 hash
			regexp.MustCompile(`(?i)^[a-f0-9]{8,}(?:-[a-f0-9]{8,})+\.js$`),
		}
	})

	for _, re := range hashedJSFilenameRe {
		if re.MatchString(filename) {
			return true
		}
	}
	return false
}

func isCacheControlStrong(cacheControl string) bool {
	cc := strings.ToLower(strings.TrimSpace(cacheControl))
	if cc == "" {
		return false
	}
	if strings.Contains(cc, "immutable") {
		return true
	}

	// 解析 max-age=xxx
	for _, part := range strings.Split(cc, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "max-age=") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(part, "max-age="))
		sec, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		// 大于等于 1 天认为是强缓存
		return sec >= 86400
	}
	return false
}

var (
	chunkStaticJSGlobsOnce sync.Once
	strongBundledJSGlobs   []glob.Glob
	weakBundledJSGlobs     []glob.Glob
)

func isBundledJavaScriptStrongPath(p string) bool {
	for _, g := range strongBundledJSGlobsList() {
		if g.Match(p) {
			return true
		}
	}
	return false
}

func isBundledJavaScriptWeakPath(p string) bool {
	for _, g := range weakBundledJSGlobsList() {
		if g.Match(p) {
			return true
		}
	}
	return false
}

func strongBundledJSGlobsList() []glob.Glob {
	chunkStaticJSGlobsOnce.Do(func() {
		strongPatterns := []string{
			// Next.js / React 常见构建产物目录
			"/_next/static/*.js",
			"/_next/static/**/*.js",
			// Webpack 常见分包目录
			"/static/chunks/*.js",
			"/static/chunks/**/*.js",
			"/static/js/*.js",
			"/static/js/**/*.js",
			// 典型命名：*.chunk.js
			"/**/*.chunk.js",
		}
		weakPatterns := []string{
			// 通用静态目录（可能包含手写 JS，因此只是“弱路径”）
			"/static/*.js",
			"/static/**/*.js",
			"/assets/*.js",
			"/assets/**/*.js",
		}

		strongBundledJSGlobs = make([]glob.Glob, 0, len(strongPatterns))
		for _, pat := range strongPatterns {
			g, err := glob.Compile(strings.ToLower(pat))
			if err != nil {
				continue
			}
			strongBundledJSGlobs = append(strongBundledJSGlobs, g)
		}

		weakBundledJSGlobs = make([]glob.Glob, 0, len(weakPatterns))
		for _, pat := range weakPatterns {
			g, err := glob.Compile(strings.ToLower(pat))
			if err != nil {
				continue
			}
			weakBundledJSGlobs = append(weakBundledJSGlobs, g)
		}
	})
	return strongBundledJSGlobs
}

func weakBundledJSGlobsList() []glob.Glob {
	_ = strongBundledJSGlobsList()
	return weakBundledJSGlobs
}
