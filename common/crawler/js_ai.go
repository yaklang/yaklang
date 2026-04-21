package crawler

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIJSExtractConfig holds runtime knobs for AI assisted JS/HTML candidate extraction.
type AIJSExtractConfig struct {
	// MaxTokens caps the size of one AI call payload. Defaults to 80K.
	MaxTokens int
	// ChunkBytes is the target byte size of each AI call slice. Defaults to 250KB.
	// Candidate blocks are packed greedily up to this budget; a single oversized
	// block may exceed it slightly but will still be token-shrunk by MaxTokens.
	ChunkBytes int64
	// OverlapBytes is how many bytes of the previous chunk are folded into the
	// current chunk via DumpWithOverlap. Defaults to 2048.
	OverlapBytes int
	// ContextBytes is the half-window size taken around each regex hit when
	// building candidate windows. Defaults to 120.
	ContextBytes int
	// SkipBelowBytes: when the candidate stream is smaller than this AND the
	// small-input direct-feed fast path is disabled (or did not apply), the
	// AI step is skipped and raw deduplicated hits are emitted directly.
	// In normal usage SmallInputBytes / SmallInputTokens take precedence and
	// this branch is rarely reached.
	SkipBelowBytes int
	// SmallInputBytes: when the raw input source is smaller than this AND
	// the token estimate is smaller than SmallInputTokens, RunAIJSExtract
	// skips the regex pre-filter entirely and feeds the full source to the
	// AI in a single call. This preserves cross-statement context (for
	// example `var x = 'deep.js'` followed by `fetch(x)`) that would
	// otherwise be lost after windowed slicing. Set to 0 to disable.
	// Defaults to 200KB.
	SmallInputBytes int
	// SmallInputTokens: companion threshold to SmallInputBytes; both must
	// be satisfied to take the direct-feed fast path. Set to 0 to disable.
	// Defaults to 50K tokens.
	SmallInputTokens int
	// Concurrency caps parallel AI calls when reducing chunks. Defaults to 2.
	Concurrency int
	// AIOptions are forwarded to the LiteForge coordinator (model/provider/etc).
	AIOptions []aicommon.ConfigOption

	// IsHTTPS records the scheme of the originating HTTP request. Together
	// with RequestRaw it is injected into every AI call so the model can
	// resolve relative paths into absolute URLs.
	IsHTTPS bool
	// RequestRaw is the raw HTTP request packet (method + URI + headers) of
	// the page that produced the JS being analyzed. Only the request head
	// (no body) is passed to the AI, and it is truncated to RequestHeadMaxBytes.
	RequestRaw []byte
	// RequestHeadMaxBytes caps how many bytes of RequestRaw are prepended to
	// each AI call payload. Defaults to 4096.
	RequestHeadMaxBytes int
}

// AIJSExtractOption mutates AIJSExtractConfig.
type AIJSExtractOption func(*AIJSExtractConfig)

// WithAIJS_MaxTokens overrides the per-call token budget.
func WithAIJS_MaxTokens(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n > 0 {
			c.MaxTokens = n
		}
	}
}

// WithAIJS_ChunkBytes overrides the target byte size of each AI call slice.
func WithAIJS_ChunkBytes(n int64) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n > 0 {
			c.ChunkBytes = n
		}
	}
}

// WithAIJS_OverlapBytes overrides the cross-chunk fold size.
func WithAIJS_OverlapBytes(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n >= 0 {
			c.OverlapBytes = n
		}
	}
}

// WithAIJS_ContextBytes overrides the half-window size around each regex hit.
func WithAIJS_ContextBytes(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n > 0 {
			c.ContextBytes = n
		}
	}
}

// WithAIJS_SkipBelowBytes sets the candidate-stream size below which the AI
// step is skipped and raw hits are emitted directly.
func WithAIJS_SkipBelowBytes(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n >= 0 {
			c.SkipBelowBytes = n
		}
	}
}

// WithAIJS_SmallInputBytes sets the raw input byte threshold for the
// direct-feed fast path. Set to 0 to disable.
func WithAIJS_SmallInputBytes(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n >= 0 {
			c.SmallInputBytes = n
		}
	}
}

// WithAIJS_SmallInputTokens sets the raw input token threshold for the
// direct-feed fast path. Set to 0 to disable.
func WithAIJS_SmallInputTokens(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n >= 0 {
			c.SmallInputTokens = n
		}
	}
}

// WithAIJS_Concurrency caps parallel AI calls.
func WithAIJS_Concurrency(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n > 0 {
			c.Concurrency = n
		}
	}
}

// WithAIJS_AIOptions forwards aicommon.ConfigOption (model, key, ...) to LiteForge.
func WithAIJS_AIOptions(opts ...aicommon.ConfigOption) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		c.AIOptions = append(c.AIOptions, opts...)
	}
}

// WithAIJS_BaseRequest attaches the originating HTTP request scheme and raw
// packet so that AI calls can resolve relative paths into absolute URLs.
func WithAIJS_BaseRequest(isHTTPS bool, requestRaw []byte) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		c.IsHTTPS = isHTTPS
		c.RequestRaw = requestRaw
	}
}

// WithAIJS_RequestHeadMaxBytes overrides the cap on how many bytes of the
// originating request head are injected into each AI call payload.
func WithAIJS_RequestHeadMaxBytes(n int) AIJSExtractOption {
	return func(c *AIJSExtractConfig) {
		if n > 0 {
			c.RequestHeadMaxBytes = n
		}
	}
}

// NewAIJSExtractConfig builds a config with sane defaults.
func NewAIJSExtractConfig(opts ...AIJSExtractOption) *AIJSExtractConfig {
	c := &AIJSExtractConfig{
		MaxTokens:           80 * 1024,
		ChunkBytes:          250 * 1024,
		OverlapBytes:        2048,
		ContextBytes:        120,
		SkipBelowBytes:      1024,
		SmallInputBytes:     200 * 1024,
		SmallInputTokens:    50 * 1024,
		Concurrency:         2,
		RequestHeadMaxBytes: 4096,
	}
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	return c
}

// --- regex-based pre-filter -------------------------------------------------

// Each regex below is intentionally loose: false positives are tolerated since
// the AI step (or the downstream NewHTTPRequest) will reject obvious garbage.
// We never use grouping in a way that would require sub-match parsing - a
// FindAllIndex on the full pattern is enough for surrounding-context capture.
//
// Order matters only weakly (overlapping windows are merged later), but we
// still keep the highest-quality, most context-rich patterns first so that the
// merged window centers on a useful anchor:
//
//  1. absolute / protocol-relative URLs (highest signal)
//  2. function-call style HTTP triggers: fetch / xhr.open / axios / new URL / ...
//  3. assignment-style fields:           url= / href= / endpoint: / baseURL: / ...
//  4. path-style strings starting with / (still very common in routing tables)
//  5. resource-suffix style:             foo.js / a/b.action
//  6. router-registry style quoted multi-segment paths
//  7. quoted file-name literals (no leading slash) - last-resort coverage for
//     tokens like 'deep.js' that get assigned to a variable and only used by
//     reference later, where no other anchor would catch them
var aiJSCandidatePatterns = []*regexp.Regexp{
	// 1. absolute and protocol-relative URLs
	regexp.MustCompile(`(?:https?://|//)[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=\-]{2,}`),

	// 2.a fetch('...')
	regexp.MustCompile(`\bfetch\s*\(\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `]`),
	// 2.b XHR-style: anything ".open('METHOD', '...')" - covers xhr.open and friends
	regexp.MustCompile(`\.open\s*\(\s*['"` + "`" + `][A-Za-z]+['"` + "`" + `]\s*,\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `]`),
	// 2.c new XMLHttpRequest / new URL('...') / new Request('...') / new WebSocket('...') / new EventSource('...')
	regexp.MustCompile(`\bnew\s+(?:XMLHttpRequest|URL|Request|WebSocket|EventSource)\b(?:\s*\(\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `])?`),
	// 2.d axios('...') / axios.get|post|put|delete|patch|head|options('...')
	regexp.MustCompile(`\baxios(?:\s*\.\s*[a-z]+)?\s*\(\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `]`),
	// 2.e other common HTTP libs: ky, got, request, superagent.<verb>
	regexp.MustCompile(`\b(?:ky|got|request|superagent(?:\s*\.\s*[a-z]+)?)\s*\(\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `]`),
	// 2.f jQuery $.get / $.post / $.ajax / $.getJSON / $.put / $.delete('...')
	regexp.MustCompile(`\$\s*\.\s*(?:get|post|ajax|getJSON|put|delete|head|patch)\s*\(\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `]`),
	// 2.g dynamic import('...') and require('...')
	regexp.MustCompile(`\b(?:import|require)\s*\(\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `]`),

	// 3. assignment-style: url|href|src|endpoint|api|apiUrl|baseURL|baseUrl|base_url|uri|path|action[: =]'value'
	regexp.MustCompile(`\b(?:url|href|src|endpoint|api|apiUrl|baseURL|baseUrl|base_url|uri|path|action)\s*[:=]\s*['"` + "`" + `][^'"` + "`" + `\r\n]{1,1000}['"` + "`" + `]`),

	// 4. path-style strings starting with /
	regexp.MustCompile(`/[A-Za-z0-9._~\-/]{2,}(?:\?[^\s'"<>` + "`" + `]{0,200})?`),
	// 5. resource-suffix style (relative or fragment paths with known extensions)
	regexp.MustCompile(`[A-Za-z0-9_\-/]{1,}\.(?:js|mjs|cjs|json|action|do|php|asp|aspx|jsp)(?:\?[^\s'"<>` + "`" + `]{0,200})?`),
	// 6. router-registry style: words with at least one slash inside quotes/backticks
	regexp.MustCompile("['\"`](?:/?[A-Za-z0-9_\\-]+){2,}['\"`]"),

	// 7. quoted file-name literals (no leading slash required) - this is what
	//    catches `var deepUrl = 'deep.js'` so the AI can later see the
	//    surrounding variable name and the call site that uses it.
	regexp.MustCompile(`['"` + "`" + `][A-Za-z0-9_\-./]{1,128}\.(?:js|mjs|cjs|jsx|ts|tsx|json|action|do|php|asp|aspx|jsp|html|htm)['"` + "`" + `]`),
}

// aiJSRawSafePatterns is the subset of aiJSCandidatePatterns whose matches are
// safe to emit as a raw path candidate (no enclosing function-call syntax,
// no assignment-prefix like "url=" leaking into the match). These are used by
// rawCandidateHits (and therefore by the direct-feed fast path) to hand
// high-confidence path / URL strings straight to NewHTTPRequest without
// going through an AI round trip.
//
// NOTE: the order here mirrors aiJSCandidatePatterns so the visual mapping
// stays obvious.
var aiJSRawSafePatterns = []*regexp.Regexp{
	// 1. absolute and protocol-relative URLs
	regexp.MustCompile(`(?:https?://|//)[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=\-]{2,}`),
	// 4. path-style strings starting with /
	regexp.MustCompile(`/[A-Za-z0-9._~\-/]{2,}(?:\?[^\s'"<>` + "`" + `]{0,200})?`),
	// 5. resource-suffix style
	regexp.MustCompile(`[A-Za-z0-9_\-/]{1,}\.(?:js|mjs|cjs|json|action|do|php|asp|aspx|jsp)(?:\?[^\s'"<>` + "`" + `]{0,200})?`),
	// 6. router-registry style: words with at least one slash inside quotes
	regexp.MustCompile("['\"`](?:/?[A-Za-z0-9_\\-]+){2,}['\"`]"),
	// 7. quoted file-name literals
	regexp.MustCompile(`['"` + "`" + `][A-Za-z0-9_\-./]{1,128}\.(?:js|mjs|cjs|jsx|ts|tsx|json|action|do|php|asp|aspx|jsp|html|htm)['"` + "`" + `]`),
}

// candidateWindow describes a single hit and the surrounding context.
type candidateWindow struct {
	matchStart int
	matchEnd   int
	winStart   int
	winEnd     int
}

// extractURLLikeCandidates scans text with a set of broad URL/path patterns
// and returns one ready-to-feed text stream. Each hit is wrapped in a clearly
// marked block so that the AI step has enough surrounding code to disambiguate
// (and so that aireducer.WithSeparatorTrigger can split on block boundaries).
//
// The returned slice contains the formatted blocks; callers usually want
// strings.Join(..., "") or to feed them line-by-line.
func extractURLLikeCandidates(text string, contextBytes int) []string {
	if text == "" {
		return nil
	}
	if contextBytes <= 0 {
		contextBytes = 120
	}

	var hits []candidateWindow
	for _, p := range aiJSCandidatePatterns {
		for _, idx := range p.FindAllStringIndex(text, -1) {
			if len(idx) < 2 {
				continue
			}
			s, e := idx[0], idx[1]
			ws := s - contextBytes
			if ws < 0 {
				ws = 0
			}
			we := e + contextBytes
			if we > len(text) {
				we = len(text)
			}
			hits = append(hits, candidateWindow{
				matchStart: s,
				matchEnd:   e,
				winStart:   ws,
				winEnd:     we,
			})
		}
	}
	if len(hits) == 0 {
		return nil
	}

	// merge overlapping windows so that nearby hits collapse into one block
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].winStart < hits[j].winStart
	})
	merged := make([]candidateWindow, 0, len(hits))
	merged = append(merged, hits[0])
	for i := 1; i < len(hits); i++ {
		last := &merged[len(merged)-1]
		cur := hits[i]
		if cur.winStart <= last.winEnd {
			if cur.winEnd > last.winEnd {
				last.winEnd = cur.winEnd
			}
			if cur.matchEnd > last.matchEnd {
				last.matchEnd = cur.matchEnd
			}
		} else {
			merged = append(merged, cur)
		}
	}

	out := make([]string, 0, len(merged))
	for _, w := range merged {
		body := text[w.winStart:w.winEnd]
		// trim partial UTF-8 and obvious binary noise; keep printable as-is
		body = strings.ReplaceAll(body, "\x00", "")
		out = append(out, fmt.Sprintf(
			"--- candidate ---\noffset=%d-%d\n%s\n--- end ---\n",
			w.matchStart, w.matchEnd, body,
		))
	}
	return out
}

// rawCandidateHits returns the raw matched substrings (deduplicated, in
// match order). Used as a fallback when input is too small for AI processing,
// and as the "raw" leg of the direct-feed fast path.
//
// Only aiJSRawSafePatterns are scanned here. The function-call style and
// assignment-style patterns in aiJSCandidatePatterns are intentionally
// skipped: their matches include the function name, method argument, or
// identifier prefix, so treating them as a raw path would produce garbage
// URLs downstream. Those patterns still contribute to extractURLLikeCandidates
// (where the surrounding window is attached before handing the slice to AI).
func rawCandidateHits(text string) []string {
	if text == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, p := range aiJSRawSafePatterns {
		for _, idx := range p.FindAllStringIndex(text, -1) {
			if len(idx) < 2 {
				continue
			}
			hit := strings.TrimSpace(text[idx[0]:idx[1]])
			// quote-wrapped patterns strip the enclosing ', ", or `
			hit = strings.Trim(hit, "'\"`")
			if hit == "" {
				continue
			}
			if _, ok := seen[hit]; ok {
				continue
			}
			seen[hit] = struct{}{}
			out = append(out, hit)
		}
	}
	return out
}

// --- candidate sanitisation -------------------------------------------------

// boundaryLeakNeedles are substrings that should never appear in a URL/path
// emitted by either the AI step or the regex fast path. They are produced by
// the crawler when concatenating HTML and JS sources and have leaked through
// in the past (e.g. "http://---html-end---/" - a regression that motivated
// this filter).
var boundaryLeakNeedles = []string{
	"yak-html-end",
	"yak-js-end",
	"---html-end---",
	"---js-chunk-end---",
	"--- candidate ---",
	"--- end ---",
}

// pathCandidateExtRe matches a known file-like extension at the end of a
// candidate, used by looksLikePathCandidate to decide whether a scheme-less
// AI output is "pathy enough" to be worth sending through NewHTTPRequest.
var pathCandidateExtRe = regexp.MustCompile(`\.(?i:js|mjs|cjs|jsx|ts|tsx|json|action|do|php|asp|aspx|jsp|html|htm|xml|txt|css|svg|png|jpg|jpeg|gif|pdf|zip|tar|gz)(?:\?.*)?$`)

// looksLikePathCandidate decides whether a scheme-less candidate plausibly
// refers to a URL path or a file. Rejects bare identifiers (HTTP methods,
// header names, HTML tag names, generic English words) that AI models often
// hallucinate as URL candidates when given free-form source, including the
// "/script", "/div", "/body" class of hallucinations where the model prefixes
// a single HTML-tag-like word with a slash.
//
// Rules:
//   - Empty or whitespace-only candidates are rejected.
//   - A candidate ending in a known file-like extension is accepted,
//     which catches bare references such as `deep.js`, `/script.js`, or
//     `list.json`.
//   - A candidate with two or more non-empty path segments is accepted
//     (e.g. `/api/users`, `a/b/c`), since a multi-segment path is unlikely
//     to be a single stray identifier.
//   - Everything else (bare identifiers "POST", "HackedJS"; single-segment
//     paths like "/script", "/div", "/body"; query-only fragments "?q=1")
//     is rejected.
func looksLikePathCandidate(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if pathCandidateExtRe.MatchString(s) {
		return true
	}
	trimmed := strings.Trim(s, "/")
	if trimmed == "" {
		return false
	}
	nonempty := 0
	for _, seg := range strings.Split(trimmed, "/") {
		if seg != "" {
			nonempty++
		}
	}
	return nonempty >= 2
}

// looksLikeBoundaryLeak returns true if the candidate string contains any
// known boundary marker. These markers are inserted by the crawler to glue
// HTML and JS blobs together and must never propagate as a real path.
func looksLikeBoundaryLeak(s string) bool {
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	for _, n := range boundaryLeakNeedles {
		if strings.Contains(low, n) {
			return true
		}
	}
	return false
}

// hostLabelRe matches a single DNS label (RFC 1035 friendly subset). We keep
// it intentionally permissive: leading/trailing dashes are rejected, an
// internal "---" is rejected (legitimate IDN xn-- prefixes use exactly two
// dashes), and the label must contain at least one alphanumeric char.
var hostLabelRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9\-]{0,61}[A-Za-z0-9])?$`)

// looksLikeValidHost returns true for plausibly real hostnames or IP literals.
// It rejects boundary-marker leaks ("---html-end---"), bare single-label
// non-localhost names ("app"), labels containing "---", and anything with
// whitespace/quotes. Used as a sanity gate over AI-emitted absolute URLs.
func looksLikeValidHost(host string) bool {
	if host == "" {
		return false
	}
	if strings.ContainsAny(host, " \t\r\n<>\"'`") {
		return false
	}
	if looksLikeBoundaryLeak(host) {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	// Multi-label hostname: must contain a dot AND every label must look DNS-y.
	if !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		if strings.Contains(label, "---") {
			return false
		}
		if !hostLabelRe.MatchString(label) {
			return false
		}
	}
	return true
}

// sanitizeAIURL validates an absolute URL string emitted by the AI step.
// On success it returns the canonical URL with fragment stripped and the
// scheme normalised to lowercase; the original query is preserved. Returns
// ("", false) if the URL is malformed, has a non-http(s) scheme, has an
// implausible host, or contains a boundary-marker leak.
func sanitizeAIURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if looksLikeBoundaryLeak(raw) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := u.Hostname()
	if !looksLikeValidHost(host) {
		return "", false
	}
	u.Scheme = scheme
	u.Fragment = ""
	return u.String(), true
}

// --- LiteForge invocation ---------------------------------------------------

const aiJSExtractPromptTpl = `# 角色
你是一名 Web 应用资产识别助手。输入由两部分组成：

1) 一个 "REQUEST CONTEXT" 块，描述当前正在被分析的页面的请求上下文，形如：

    === REQUEST CONTEXT ===
    scheme: https
    base_url: https://example.com/app/index.html
    host: example.com
    request_head:
    GET /app/index.html HTTP/1.1
    Host: example.com
    ...
    === END REQUEST CONTEXT ===

2) 若干 "candidate" 窗口，每个窗口从 JavaScript / HTML 源码里预筛选得到：

    --- candidate ---
    offset=START-END
    <surrounding code with one URL/path-like hit inside>
    --- end ---

# 任务
基于 REQUEST CONTEXT 里的 scheme / base_url / host，识别候选窗口中"业务可访问"的 URL，并**直接输出完整的 http:// 或 https:// URL**（含 scheme 和 host）。

# 拼接规则
- 相对路径（以 "/" 开头）：用 REQUEST CONTEXT 的 scheme + host 拼接成 scheme://host/path
- 相对路径（不以 "/" 开头，如 "static/app.js"）：基于 base_url 目录拼接
- 协议相对 URL（以 "//" 开头）：补上 REQUEST CONTEXT 的 scheme
- 已经是完整 URL（含 http:// 或 https://）：保持原样
- 保留 query string，不改写

# 必须剔除
- 注释、版本号、UUID、CSS 选择器、字体/图片/音视频静态资源
- mailto: / tel: / javascript: / data: / blob: 等非 HTTP 协议
- #fragment 锚点
- 第三方公共 CDN（jsdelivr/unpkg/cdnjs/google-analytics 等）
- 模板占位符未替换的字符串（含 ${...} {{...}} :param 等）

# 注意
- 同一 URL 只输出一次
- 不要 url-encode
- 如果候选窗口里没有可信路径，输出空数组
- 对不确定能否拼到完整 host 的，丢弃而不是猜测`

// buildRequestContextBlock renders the REQUEST CONTEXT header that is
// prepended to every AI slice payload. It lets the model resolve relative
// paths into absolute URLs using the scheme / base_url / host of the
// originating HTTP request. Returns an empty string when no request context
// is available, so the legacy "paths only" behavior is preserved.
func buildRequestContextBlock(cfg *AIJSExtractConfig) string {
	if cfg == nil || len(cfg.RequestRaw) == 0 {
		return ""
	}

	scheme := "http"
	if cfg.IsHTTPS {
		scheme = "https"
	}
	baseURL := lowhttp.GetUrlFromHTTPRequest(scheme, cfg.RequestRaw)
	host := lowhttp.GetHTTPPacketHeader(cfg.RequestRaw, "Host")

	// strip body; only send request head (method + URI + headers) to the AI
	headers, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(cfg.RequestRaw)
	headers = strings.TrimRight(headers, "\r\n")

	limit := cfg.RequestHeadMaxBytes
	if limit <= 0 {
		limit = 4096
	}
	if len(headers) > limit {
		headers = headers[:limit] + "\n... (truncated)"
	}

	var b strings.Builder
	b.WriteString("=== REQUEST CONTEXT ===\n")
	b.WriteString("scheme: ")
	b.WriteString(scheme)
	b.WriteByte('\n')
	if baseURL != "" {
		b.WriteString("base_url: ")
		b.WriteString(baseURL)
		b.WriteByte('\n')
	}
	if host != "" {
		b.WriteString("host: ")
		b.WriteString(host)
		b.WriteByte('\n')
	}
	b.WriteString("request_head:\n")
	b.WriteString(headers)
	b.WriteString("\n=== END REQUEST CONTEXT ===\n\n")
	return b.String()
}

// pathExtractFunc is the function-pointer indirection used by RunAIJSExtract
// so that tests can swap the AI call for a deterministic stub. Production code
// always points to invokeLiteForgeForPaths.
type pathExtractFunc func(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error

var invokeLiteForgeForPathsFunc pathExtractFunc = invokeLiteForgeForPaths

// invokeLiteForgeForPaths runs one LiteForge call against a single payload
// and emits each accepted path through onPath. Errors are logged and swallowed
// so a single failed slice does not abort the whole reducer.
func invokeLiteForgeForPaths(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) error {
	if onPath == nil {
		return nil
	}
	if strings.TrimSpace(payload) == "" {
		return nil
	}

	forge, err := aiforge.NewLiteForge(
		"crawler-js-path-extract",
		aiforge.WithLiteForge_Prompt(aiJSExtractPromptTpl),
		aiforge.WithLiteForge_SpeedPriority(true),
		aiforge.WithLiteForge_OutputSchema(
			aitool.WithStructArrayParam(
				"urls",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("Absolute URLs identified from candidate windows"),
				},
				nil,
				aitool.WithStringParam("value",
					aitool.WithParam_Description("Absolute URL starting with http:// or https://"),
				),
			),
		),
		aiforge.WithExtendLiteForge_AIOption(cfg.AIOptions...),
	)
	if err != nil {
		return utils.Errorf("build liteforge failed: %v", err)
	}

	result, err := forge.Execute(ctx, []*ypb.ExecParamItem{
		{Key: "candidates", Value: payload},
	})
	if err != nil {
		log.Warnf("ai js extract liteforge execute failed: %v", err)
		return nil
	}
	if result == nil || result.Action == nil {
		log.Warn("ai js extract liteforge returned empty action")
		return nil
	}

	items := result.GetInvokeParamsArray("urls")
	for _, item := range items {
		raw := strings.TrimSpace(item.GetString("value"))
		if raw == "" {
			continue
		}
		cleaned, ok := sanitizeAIURL(raw)
		if !ok {
			log.Debugf("ai js extract: drop invalid AI url candidate: %q", raw)
			continue
		}
		log.Infof("AI found url in JS: %s", cleaned)
		onPath(cleaned)
	}
	return nil
}

// --- public entry -----------------------------------------------------------

// RunAIJSExtract drives the extraction pipeline. It picks one of two paths:
//
//   - direct-feed fast path: when the raw input is small (under both
//     SmallInputBytes and SmallInputTokens) the entire source is fed to
//     the AI in one call, with no regex pre-filter. This preserves
//     cross-statement context such as variable assignments referenced by
//     a later fetch() call. This is the default for all small inputs.
//
//   - regex + reducer slow path: for larger inputs we run the broad regex
//     pre-filter to build candidate windows, slice them with aireducer
//     using DumpWithOverlap folding, and run LiteForge SpeedPriority
//     extraction per slice in parallel.
//
// Each accepted path is emitted through onPath. The function never returns
// the AI errors of an individual slice - it logs and continues, so the
// upstream crawler keeps running.
func RunAIJSExtract(ctx context.Context, code string, cfg *AIJSExtractConfig, onPath func(string)) error {
	if onPath == nil {
		return utils.Error("ai js extract: onPath is nil")
	}
	if cfg == nil {
		cfg = NewAIJSExtractConfig()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// emit canonicalises and dedupes a candidate produced by either the AI
	// step or the regex fast path. The rules:
	//
	//   * boundary-marker leaks ("yak-html-end", "---html-end---", ...) are
	//     always dropped (regression: leaked as "http://---html-end---/").
	//   * candidates that declare a URI scheme (anything with ":" before the
	//     first "/") MUST be a valid http(s) URL with a plausible host; this
	//     blocks "javascript:", "mailto:", "data:", garbage hosts, and also
	//     strips #fragments.
	//   * scheme-less candidates (relative paths like "/api/v1") are passed
	//     through unchanged so the downstream NewHTTPRequest can resolve
	//     them against the originating page.
	var emitMu sync.Mutex
	emitted := make(map[string]struct{})
	emit := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if looksLikeBoundaryLeak(raw) {
			log.Debugf("ai js extract: drop boundary-marker leak: %q", raw)
			return
		}

		canonical := raw
		schemeless := true
		// Detect a URI scheme: ':' present AND no '/' precedes it. This
		// avoids mis-identifying paths like "a.js?x=1:b" as a scheme.
		if colon := strings.IndexByte(raw, ':'); colon > 0 {
			if slash := strings.IndexByte(raw, '/'); slash < 0 || slash > colon {
				cleaned, ok := sanitizeAIURL(raw)
				if !ok {
					log.Debugf("ai js extract: drop invalid scheme/url candidate: %q", raw)
					return
				}
				canonical = cleaned
				schemeless = false
			}
		}
		if schemeless && !looksLikePathCandidate(canonical) {
			// AI models occasionally hallucinate bare identifiers as URL
			// candidates (HTTP methods like "POST", header names like
			// "HackedJS", HTML tag names like "div"/"body"/"script"). Drop
			// anything that does not look like a path or file reference.
			log.Debugf("ai js extract: drop non-pathy scheme-less candidate: %q", canonical)
			return
		}

		emitMu.Lock()
		defer emitMu.Unlock()
		if _, ok := emitted[canonical]; ok {
			return
		}
		emitted[canonical] = struct{}{}
		onPath(canonical)
	}

	// Direct-feed fast path: small enough to fit in one AI call without
	// losing context. This is what makes simple SPAs (a handful of small
	// JS files) work well - chopping them into windowed candidates would
	// strip the surrounding variable assignments and call sites the AI
	// needs to resolve a relative path against the page's base_url.
	//
	// We use a hybrid strategy here:
	//
	//   1. emit raw regex hits up front so high-confidence path / file-name
	//      candidates (e.g. `var deepUrl = 'deep.js'`) reach the crawler
	//      even if the AI step decides to omit them. The downstream
	//      NewHTTPRequest resolves bare relative paths against the page's
	//      base_url, so a hit like "deep.js" still becomes a real URL.
	//
	//   2. then call the AI on the full code so the model can also surface
	//      structurally-implied URLs (e.g. an inline fetch with a string
	//      literal that the regex might have missed) and dedup naturally
	//      thanks to the shared `emit` closure.
	if cfg.SmallInputBytes > 0 && cfg.SmallInputTokens > 0 &&
		len(code) > 0 &&
		len(code) < cfg.SmallInputBytes &&
		aicommon.MeasureTokens(code) < cfg.SmallInputTokens {
		for _, hit := range rawCandidateHits(code) {
			emit(hit)
		}

		payload := buildRequestContextBlock(cfg) + code
		if cfg.MaxTokens > 0 {
			if aicommon.MeasureTokens(payload) > cfg.MaxTokens {
				payload = aicommon.ShrinkTextBlockByTokens(payload, cfg.MaxTokens)
			}
		}
		log.Debugf("ai js extract: small input bytes=%d, direct-feed fast path", len(code))
		_ = invokeLiteForgeForPathsFunc(ctx, cfg, payload, emit)
		return nil
	}

	candidates := extractURLLikeCandidates(code, cfg.ContextBytes)
	if len(candidates) == 0 {
		log.Debug("ai js extract: no candidates from regex pre-filter")
		return nil
	}

	// concatenate candidate windows once; aireducer separator trigger will
	// split exactly on block boundaries when possible
	var streamBuf bytes.Buffer
	for _, c := range candidates {
		streamBuf.WriteString(c)
	}
	stream := streamBuf.String()

	if streamBuf.Len() < cfg.SkipBelowBytes {
		for _, hit := range rawCandidateHits(code) {
			emit(hit)
		}
		log.Debugf("ai js extract: stream %v < skip threshold %v, fast path", streamBuf.Len(), cfg.SkipBelowBytes)
		return nil
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	swg := utils.NewSizedWaitGroup(concurrency)

	reducer, err := aireducer.NewReducerFromString(
		stream,
		aireducer.WithContext(ctx),
		aireducer.WithChunkSize(cfg.ChunkBytes),
		aireducer.WithSeparatorTrigger("\n--- end ---\n"),
		// Pack candidate blocks to fill ChunkBytes instead of emitting one
		// chunk per candidate - the "--- end ---" separator only acts as a
		// preferred cut boundary within the chunkSize window.
		aireducer.WithSeparatorAsBoundary(true),
		aireducer.WithReducerCallback(func(rcfg *aireducer.Config, _ *aid.PromptContextProvider, ch chunkmaker.Chunk) error {
			body := ch.DumpWithOverlap(cfg.OverlapBytes)
			payload := buildRequestContextBlock(cfg) + body
			if cfg.MaxTokens > 0 {
				if aicommon.MeasureTokens(payload) > cfg.MaxTokens {
					payload = aicommon.ShrinkTextBlockByTokens(payload, cfg.MaxTokens)
				}
			}
			log.Debugf("ai js extract: slice payload bytes=%d (chunk bytes=%d)", len(payload), ch.BytesSize())

			swg.Add(1)
			go func() {
				defer swg.Done()
				_ = invokeLiteForgeForPathsFunc(ctx, cfg, payload, emit)
			}()
			return nil
		}),
	)
	if err != nil {
		return utils.Errorf("ai js extract: build reducer failed: %v", err)
	}

	if err := reducer.Run(); err != nil {
		log.Warnf("ai js extract: reducer run failed: %v", err)
	}
	swg.Wait()
	return nil
}
