package crawler

import (
	"bytes"
	"context"
	"fmt"
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
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIJSExtractConfig holds runtime knobs for AI assisted JS/HTML candidate extraction.
type AIJSExtractConfig struct {
	// MaxTokens caps the size of one AI call payload. Defaults to 80K.
	MaxTokens int
	// ChunkBytes is the hard byte limit of each aireducer slice. Defaults to ~320KB
	// (≈80K tokens * 4 bytes per token, a coarse upper bound for english code).
	ChunkBytes int64
	// OverlapBytes is how many bytes of the previous chunk are folded into the
	// current chunk via DumpWithOverlap. Defaults to 2048.
	OverlapBytes int
	// ContextBytes is the half-window size taken around each regex hit when
	// building candidate windows. Defaults to 120.
	ContextBytes int
	// SkipBelowBytes: when the candidate stream is smaller than this, the AI
	// step is skipped and raw deduplicated hits are emitted directly.
	SkipBelowBytes int
	// Concurrency caps parallel AI calls when reducing chunks. Defaults to 2.
	Concurrency int
	// AIOptions are forwarded to the LiteForge coordinator (model/provider/etc).
	AIOptions []aicommon.ConfigOption
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

// WithAIJS_ChunkBytes overrides the byte size of each reducer slice.
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

// NewAIJSExtractConfig builds a config with sane defaults.
func NewAIJSExtractConfig(opts ...AIJSExtractOption) *AIJSExtractConfig {
	c := &AIJSExtractConfig{
		MaxTokens:      80 * 1024,
		ChunkBytes:     320 * 1024,
		OverlapBytes:   2048,
		ContextBytes:   120,
		SkipBelowBytes: 1024,
		Concurrency:    2,
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
var aiJSCandidatePatterns = []*regexp.Regexp{
	// absolute and protocol-relative URLs
	regexp.MustCompile(`(?:https?://|//)[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=\-]{2,}`),
	// path-style strings starting with /
	regexp.MustCompile(`/[A-Za-z0-9._~\-/]{2,}(?:\?[^\s'"<>` + "`" + `]{0,200})?`),
	// resource-suffix style (relative or fragment paths with known extensions)
	regexp.MustCompile(`[A-Za-z0-9_\-/]{1,}\.(?:js|mjs|cjs|json|action|do|php|asp|aspx|jsp)(?:\?[^\s'"<>` + "`" + `]{0,200})?`),
	// router-registry style: words with at least one slash inside quotes/backticks
	regexp.MustCompile("['\"`](?:/?[A-Za-z0-9_\\-]+){2,}['\"`]"),
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
// match order). Used as a fallback when input is too small for AI processing.
func rawCandidateHits(text string) []string {
	if text == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, p := range aiJSCandidatePatterns {
		for _, idx := range p.FindAllStringIndex(text, -1) {
			if len(idx) < 2 {
				continue
			}
			hit := strings.TrimSpace(text[idx[0]:idx[1]])
			// router-registry pattern wraps with quotes/backticks - strip them
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

// --- LiteForge invocation ---------------------------------------------------

const aiJSExtractPromptTpl = `# 角色
你是一名 Web 应用资产识别助手。下面会提供从一段 JavaScript 或 HTML 文本中预筛选出的若干"可疑窗口"，每个窗口形如：

    --- candidate ---
    offset=START-END
    <surrounding code with one URL/path-like hit inside>
    --- end ---

# 任务
仅识别"业务可访问"的相对路径或绝对 URL，并按 SCHEMA 输出。

# 必须剔除
- 注释、版本号、UUID、CSS 选择器、字体/图片/音视频静态资源
- mailto:/tel:/javascript:/data:/blob: 等非 HTTP 协议
- #fragment 锚点
- 第三方公共 CDN（如 jsdelivr/unpkg/cdnjs/google-analytics 等）
- 模板占位符未替换的字符串（含 ${...} {{...}} :param 等）

# 注意
- 同一路径只输出一次
- 输出原样字符串，不要补全前缀，不要 url-encode
- 路径风格放在 kind=path，含 scheme/host 的放在 kind=url
- 如果窗口里没有可信路径，不要硬编`

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
				"paths",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("Paths or URLs identified from candidate windows"),
				},
				nil,
				aitool.WithStringParam("value",
					aitool.WithParam_Description("Raw path or URL string"),
				),
				aitool.WithStringParam("kind",
					aitool.WithParam_EnumString("path", "url"),
					aitool.WithParam_Description("path = relative path; url = absolute URL with scheme"),
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

	items := result.GetInvokeParamsArray("paths")
	for _, item := range items {
		val := strings.TrimSpace(item.GetString("value"))
		if val == "" {
			continue
		}
		onPath(val)
	}
	return nil
}

// --- public entry -----------------------------------------------------------

// RunAIJSExtract drives the three-stage pipeline:
//  1. broad regex pre-filter to candidate windows
//  2. aireducer slicing with DumpWithOverlap folding
//  3. LiteForge SpeedPriority extraction per slice
//
// Each accepted path is emitted through onPath. The function never returns the
// AI errors of an individual slice - it logs and continues, so the upstream
// crawler keeps running.
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
		seen := make(map[string]struct{})
		for _, hit := range rawCandidateHits(code) {
			if _, ok := seen[hit]; ok {
				continue
			}
			seen[hit] = struct{}{}
			onPath(hit)
		}
		log.Debugf("ai js extract: stream %v < skip threshold %v, fast path", streamBuf.Len(), cfg.SkipBelowBytes)
		return nil
	}

	// concurrency-bounded onPath wrapper with dedup so that overlap fold does
	// not emit the same path multiple times
	var emitMu sync.Mutex
	emitted := make(map[string]struct{})
	emit := func(p string) {
		emitMu.Lock()
		defer emitMu.Unlock()
		if _, ok := emitted[p]; ok {
			return
		}
		emitted[p] = struct{}{}
		onPath(p)
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
		aireducer.WithReducerCallback(func(rcfg *aireducer.Config, _ *aid.PromptContextProvider, ch chunkmaker.Chunk) error {
			payload := ch.DumpWithOverlap(cfg.OverlapBytes)
			if cfg.MaxTokens > 0 {
				if aicommon.MeasureTokens(payload) > cfg.MaxTokens {
					payload = aicommon.ShrinkTextBlockByTokens(payload, cfg.MaxTokens)
				}
			}
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
