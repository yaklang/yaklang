package crawler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

// AIJSAsset keeps the source boundary for one HTML, JavaScript, route, or
// configuration asset. Keeping assets separate is important for large bundles:
// the AI receives a bounded evidence window and its source URL, never an
// undifferentiated concatenation of every response discovered on the page.
type AIJSAsset struct {
	SourceURL   string
	ContentType string
	Body        string
}

// AIJSExtractEvent describes one local trigger decision. It deliberately
// contains metadata only; source text and request credentials are never copied
// into observers.
type AIJSExtractEvent struct {
	SourceURL       string
	ContentType     string
	SourceBytes     int
	TriggerScore    int
	TriggerSignals  []string
	Triggered       bool
	RawCandidates   int
	CandidateBlocks int
	AIRequests      int
	Reason          string
}

// AIJSExtractObserver receives per-asset trigger decisions. Callers must be
// prepared for concurrent callbacks when the crawler processes pages in
// parallel.
type AIJSExtractObserver func(AIJSExtractEvent)

// AIJSInvoker is the narrow seam between deterministic candidate preparation
// and the model provider. Production uses LiteForge. Tests can install a
// context-scoped implementation without mutating process-global state.
type AIJSInvoker func(
	ctx context.Context,
	cfg *AIJSExtractConfig,
	payload string,
	onPath func(string),
) error

type aiJSInvokerContextKey struct{}

// WithAIJSInvokerContext attaches a crawler-local AI implementation to ctx.
// It is intentionally a Go-only API and is not exported to the Yak language.
// This keeps CI deterministic while allowing the AID tool to pass the same
// context through crawler.context(CTX).
func WithAIJSInvokerContext(ctx context.Context, invoker AIJSInvoker) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if invoker == nil {
		return ctx
	}
	return context.WithValue(ctx, aiJSInvokerContextKey{}, invoker)
}

type aiJSCallBudget struct {
	max  int64
	used atomic.Int64
}

func newAIJSCallBudget(max int) *aiJSCallBudget {
	return &aiJSCallBudget{max: int64(max)}
}

func (b *aiJSCallBudget) take() bool {
	if b == nil {
		return false
	}
	if b.max <= 0 {
		b.used.Add(1)
		return true
	}
	for {
		used := b.used.Load()
		if used >= b.max {
			return false
		}
		if b.used.CompareAndSwap(used, used+1) {
			return true
		}
	}
}

type aiJSTriggerAssessment struct {
	score   int
	signals []string
}

var aiJSTriggerPatternSpecs = []aiJSPatternSpec{
	{
		name:  "request-sink-call",
		expr:  `(?<![A-Za-z0-9_$])(?:fetch|\$fetch|sendBeacon|importScripts|axios(?:\s*\.\s*[A-Za-z_$][A-Za-z0-9_$]*)?|ky(?:\s*\.\s*[A-Za-z_$][A-Za-z0-9_$]*)?|got|request)(?![A-Za-z0-9_$])\s*\(`,
		gates: []string{"fetch", "$fetch", "sendBeacon", "importScripts", "axios", "ky", "got", "request"},
	},
	{name: "request-sink-open", expr: `\.\s*open(?![A-Za-z0-9_$])\s*\(`, gates: []string{"open"}},
	{
		name:  "request-sink-channel",
		expr:  `(?<![A-Za-z0-9_$])new\s+(?:WebSocket|EventSource|Worker|SharedWorker|Request|URL)(?![A-Za-z0-9_$])\s*\(`,
		gates: []string{"new", "WebSocket", "EventSource", "Worker", "SharedWorker", "Request", "URL"},
	},
	{
		name:  "request-sink-worker",
		expr:  `(?<![A-Za-z0-9_$])(?:serviceWorker\s*\.\s*register|importScripts)(?![A-Za-z0-9_$])\s*\(`,
		gates: []string{"serviceWorker", "importScripts"},
	},
	{
		name:  "dynamic-request-call",
		expr:  `(?<![A-Za-z0-9_$])(?:fetch|\$fetch|sendBeacon|importScripts|axios(?:\s*\.\s*[A-Za-z_$][A-Za-z0-9_$]*)?|ky(?:\s*\.\s*[A-Za-z_$][A-Za-z0-9_$]*)?|got|request)(?![A-Za-z0-9_$])\s*\(\s*(?!['"\x60)]|$)(?=\S)`,
		gates: []string{"fetch", "$fetch", "sendBeacon", "importScripts", "axios", "ky", "got", "request"},
	},
	{
		name:  "dynamic-request-open",
		expr:  `\.\s*open(?![A-Za-z0-9_$])\s*\(\s*[^,\r\n]{0,32},\s*(?!['"\x60)]|$)(?=\S)`,
		gates: []string{"open"},
	},
	{
		name:  "dynamic-request-channel",
		expr:  `(?<![A-Za-z0-9_$])new\s+(?:WebSocket|EventSource|Worker|SharedWorker|Request|URL)(?![A-Za-z0-9_$])\s*\(\s*(?!['"\x60)]|$)(?=\S)`,
		gates: []string{"WebSocket", "EventSource", "Worker", "SharedWorker", "Request", "URL"},
	},
	{
		name:  "dynamic-module",
		expr:  `(?<![A-Za-z0-9_$])(?:import|require)(?![A-Za-z0-9_$])\s*\(\s*(?!['"\x60)]|$)(?=\S)`,
		gates: []string{"import", "require"},
	},
	{
		name:  "route-or-config",
		expr:  `(?<![A-Za-z0-9_$])(?:runtime[-_]?config|routes?\.json|routeconfig|route_config|baseurl|base_url|apiurl|api_url|apibase|api_base|endpoint|serviceendpoint|asset[-_]?manifest|\.config)(?![A-Za-z0-9_$])`,
		gates: []string{"runtime", "route", "routes", "routeconfig", "route_config", "baseurl", "base_url", "apiurl", "api_url", "apibase", "api_base", "endpoint", "serviceendpoint", "manifest", ".config"},
	},
	{
		name:  "route-fields",
		expr:  `(?m)(?:^|[\r\n;,{}])\s*(base|version|resource|action|method|next)\s*[:=]`,
		gates: []string{"base", "version", "resource", "action", "method", "next"},
	},
	{name: "string-assembly", expr: `\.\s*(?:join|concat)(?![A-Za-z0-9_$])\s*\(`, gates: []string{"join", "concat"}},
	{
		name:  "encoded-expression",
		expr:  `(?<![A-Za-z0-9_$])(?:(?:String\s*\.\s*)?fromCharCode|atob|decodeURIComponent|unescape|charCodeAt|eval)(?![A-Za-z0-9_$])\s*\(|(?<![A-Za-z0-9_$])new\s+Function(?![A-Za-z0-9_$])\s*\(`,
		gates: []string{"fromCharCode", "atob", "decodeURIComponent", "unescape", "charCodeAt", "eval", "Function"},
	},
	{
		name:  "compiled-chunk",
		expr:  `(?<![A-Za-z0-9_$])(?:webpackChunk[A-Za-z0-9_$]*|__webpack_require__|sourceMappingURL\s*=|import\s*\.\s*meta\s*\.\s*url|document\s*\.\s*currentScript|chunkFilename)(?![A-Za-z0-9_$])`,
		gates: []string{"webpackChunk", "__webpack_require__", "sourceMappingURL", "import", "document", "chunkFilename"},
	},
}

var (
	aiJSTriggerPatternsOnce sync.Once
	aiJSTriggerPatterns     *aiJSPatternSet
)

func getAIJSTriggerPatterns() *aiJSPatternSet {
	aiJSTriggerPatternsOnce.Do(func() {
		aiJSTriggerPatterns = mustAIJSPatternSet(aiJSTriggerPatternSpecs)
	})
	return aiJSTriggerPatterns
}

type aiJSTriggerLexicalInfo struct {
	code              string
	hasEncodedEscape  bool
	hasStringAssembly bool
}

// normalizeAIJSTriggerCode keeps executable tokens while masking comments and
// string bodies. Triggering on raw source makes documentation strings and
// comments indistinguishable from live calls. Bracket-member calls are
// canonicalised (obj["fetch"](...) -> obj.fetch(...)) so minified bundles do
// not evade the identifier-boundary checks used for dotted calls.
func normalizeAIJSTriggerCode(code string) aiJSTriggerLexicalInfo {
	var info aiJSTriggerLexicalInfo
	out := make([]byte, 0, len(code))

	for i := 0; i < len(code); {
		switch {
		case code[i] == '/' && i+1 < len(code) && code[i+1] == '/':
			out = append(out, ' ')
			i += 2
			for i < len(code) && code[i] != '\n' && code[i] != '\r' {
				i++
			}
		case code[i] == '/' && i+1 < len(code) && code[i+1] == '*':
			out = append(out, ' ')
			i += 2
			for i < len(code) {
				if i+1 < len(code) && code[i] == '*' && code[i+1] == '/' {
					i += 2
					break
				}
				if code[i] == '\n' || code[i] == '\r' {
					out = append(out, code[i])
				}
				i++
			}
		case code[i] == '/' && canStartAIJSRegexLiteral(code, i):
			end, ok := parseAIJSRegexLiteral(code, i)
			if !ok {
				out = append(out, code[i])
				i++
				continue
			}
			out = append(out, ' ')
			i = end
		case code[i] == '[' && hasAIJSMemberReceiver(out):
			member, end, encoded, ok := parseAIJSBracketCall(code, i)
			if !ok {
				out = append(out, code[i])
				i++
				continue
			}
			out = append(out, '.')
			out = append(out, member...)
			info.hasEncodedEscape = info.hasEncodedEscape || encoded
			i = end
		case code[i] == '\'' || code[i] == '"' || code[i] == '`':
			_, end, encoded, templateAssembly := parseAIJSQuotedString(code, i)
			info.hasEncodedEscape = info.hasEncodedEscape || encoded
			info.hasStringAssembly = info.hasStringAssembly || templateAssembly
			out = append(out, ' ')
			for ; i < end; i++ {
				if code[i] == '\n' || code[i] == '\r' {
					out = append(out, code[i])
				}
			}
		case code[i] == '\\':
			decoded, width, ok := decodeAIJSHexEscape(code[i:])
			if !ok {
				out = append(out, code[i])
				i++
				continue
			}
			out = append(out, decoded...)
			info.hasEncodedEscape = true
			i += width
		default:
			if code[i] == '+' && (i == 0 || code[i-1] != '+') && (i+1 == len(code) || code[i+1] != '+') {
				info.hasStringAssembly = true
			}
			out = append(out, code[i])
			i++
		}
	}

	info.code = string(out)
	return info
}

func canStartAIJSRegexLiteral(code string, slash int) bool {
	if slash < 0 || slash >= len(code) || code[slash] != '/' {
		return false
	}
	index := slash - 1
	for index >= 0 && isAIJSWhitespace(code[index]) {
		index--
	}
	if index < 0 {
		return true
	}
	if strings.ContainsRune(`=([{,:;!?&|+-*%^~<>`, rune(code[index])) {
		return true
	}
	if !isAIJSIdentifierByte(code[index]) {
		return false
	}
	end := index + 1
	for index >= 0 && isAIJSIdentifierByte(code[index]) {
		index--
	}
	keyword := strings.ToLower(code[index+1 : end])
	switch keyword {
	case "await", "case", "delete", "in", "instanceof", "new", "return", "throw", "typeof", "void", "yield":
		return true
	default:
		return false
	}
}

func parseAIJSRegexLiteral(code string, start int) (int, bool) {
	if start < 0 || start >= len(code) || code[start] != '/' {
		return start, false
	}
	inClass := false
	for index := start + 1; index < len(code); index++ {
		switch code[index] {
		case '\n', '\r':
			return start, false
		case '\\':
			index++
			if index >= len(code) {
				return start, false
			}
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			index++
			for index < len(code) && (code[index] >= 'a' && code[index] <= 'z' || code[index] >= 'A' && code[index] <= 'Z') {
				index++
			}
			return index, true
		}
	}
	return start, false
}

func hasAIJSMemberReceiver(code []byte) bool {
	for i := len(code) - 1; i >= 0; i-- {
		if isAIJSWhitespace(code[i]) {
			continue
		}
		return isAIJSIdentifierByte(code[i]) || code[i] == ')' || code[i] == ']'
	}
	return false
}

func parseAIJSBracketCall(code string, start int) (string, int, bool, bool) {
	i := start + 1
	for i < len(code) && isAIJSWhitespace(code[i]) {
		i++
	}
	if i >= len(code) || code[i] != '\'' && code[i] != '"' {
		return "", start, false, false
	}
	member, end, encoded, _ := parseAIJSQuotedString(code, i)
	i = end
	for i < len(code) && isAIJSWhitespace(code[i]) {
		i++
	}
	if i >= len(code) || code[i] != ']' || !isAIJSIdentifier(member) {
		return "", start, false, false
	}
	closeBracket := i
	i++
	for i < len(code) && isAIJSWhitespace(code[i]) {
		i++
	}
	if i >= len(code) || code[i] != '(' {
		return "", start, false, false
	}
	return member, closeBracket + 1, encoded, true
}

func parseAIJSQuotedString(code string, start int) (string, int, bool, bool) {
	quote := code[start]
	var decoded strings.Builder
	encoded := false
	templateAssembly := false
	for i := start + 1; i < len(code); {
		if code[i] == quote {
			return decoded.String(), i + 1, encoded, templateAssembly
		}
		if quote == '`' && code[i] == '$' && i+1 < len(code) && code[i+1] == '{' {
			templateAssembly = true
		}
		if code[i] != '\\' {
			decoded.WriteByte(code[i])
			i++
			continue
		}
		if value, width, ok := decodeAIJSHexEscape(code[i:]); ok {
			decoded.WriteString(value)
			encoded = true
			i += width
			continue
		}
		if i+1 >= len(code) {
			decoded.WriteByte(code[i])
			i++
			continue
		}
		escaped := code[i+1]
		switch escaped {
		case 'n':
			decoded.WriteByte('\n')
		case 'r':
			decoded.WriteByte('\r')
		case 't':
			decoded.WriteByte('\t')
		default:
			decoded.WriteByte(escaped)
		}
		i += 2
	}
	return decoded.String(), len(code), encoded, templateAssembly
}

func decodeAIJSHexEscape(code string) (string, int, bool) {
	if len(code) >= 4 && code[0] == '\\' && (code[1] == 'x' || code[1] == 'X') {
		if value, ok := parseAIJSHex(code[2:4]); ok {
			return string(rune(value)), 4, true
		}
	}
	if len(code) >= 6 && code[0] == '\\' && (code[1] == 'u' || code[1] == 'U') {
		if value, ok := parseAIJSHex(code[2:6]); ok {
			return string(rune(value)), 6, true
		}
	}
	return "", 0, false
}

func parseAIJSHex(code string) (int, bool) {
	value := 0
	for i := 0; i < len(code); i++ {
		value <<= 4
		switch {
		case code[i] >= '0' && code[i] <= '9':
			value += int(code[i] - '0')
		case code[i] >= 'a' && code[i] <= 'f':
			value += int(code[i]-'a') + 10
		case code[i] >= 'A' && code[i] <= 'F':
			value += int(code[i]-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func isAIJSWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isAIJSIdentifierByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '$'
}

func isAIJSIdentifier(value string) bool {
	if value == "" || !(value[0] >= 'a' && value[0] <= 'z' || value[0] >= 'A' && value[0] <= 'Z' || value[0] == '_' || value[0] == '$') {
		return false
	}
	for i := 1; i < len(value); i++ {
		if !isAIJSIdentifierByte(value[i]) {
			return false
		}
	}
	return true
}

func assessAIJSTrigger(code, sourceURL, contentType string) aiJSTriggerAssessment {
	if strings.TrimSpace(code) == "" {
		return aiJSTriggerAssessment{}
	}

	lexical := normalizeAIJSTriggerCode(code)
	normalized := lexical.code
	normalizedBytes := []byte(normalized)
	triggerPatterns := getAIJSTriggerPatterns()
	matched := triggerPatterns.matchedNames(normalizedBytes)
	seen := make(map[string]struct{})
	assessment := aiJSTriggerAssessment{}
	add := func(name string, score int) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		assessment.signals = append(assessment.signals, name)
		assessment.score += score
	}
	if matched[aiJSPCREEngineErrorSignal] {
		add("matcher-engine-error", 3)
	}

	// A literal fetch('/api/x') is already handled by deterministic extraction.
	// The small score records the sink, while expressions receive the decisive
	// score that starts semantic analysis.
	if matched["request-sink-call"] || matched["request-sink-open"] ||
		matched["request-sink-channel"] || matched["request-sink-worker"] {
		add("request-sink", 1)
	}
	if matched["dynamic-request-call"] || matched["dynamic-request-open"] ||
		matched["dynamic-request-channel"] {
		add("dynamic-request-expression", 3)
	}
	if matched["dynamic-module"] {
		add("dynamic-module-expression", 3)
	}

	if matched["route-or-config"] {
		add("route-or-config", 1)
	}

	fields := make(map[string]struct{})
	if matched["route-fields"] {
		for _, field := range triggerPatterns.pattern("route-fields").captureStrings(normalizedBytes, 1, 32) {
			fields[strings.ToLower(field)] = struct{}{}
		}
	}
	if len(fields) >= 3 {
		add("route-field-composition", 2)
	}

	if lexical.hasStringAssembly || matched["string-assembly"] {
		add("string-assembly", 2)
	}

	if lexical.hasEncodedEscape || matched["encoded-expression"] {
		add("encoded-or-obfuscated", 2)
	}

	if matched["compiled-chunk"] {
		add("compiled-chunk-runtime", 1)
	}

	// The source metadata is intentionally not enough to start AI by itself.
	// It only makes a config-like file with an unresolved expression slightly
	// more explicit in diagnostics.
	metadata := strings.ToLower(sourceURL + " " + contentType)
	if strings.Contains(metadata, ".config") || strings.Contains(metadata, "routes") ||
		strings.Contains(metadata, "manifest") || strings.Contains(metadata, "yaml") {
		add("config-asset", 1)
	}

	return assessment
}

func configuredAIJSInvoker(ctx context.Context, cfg *AIJSExtractConfig) AIJSInvoker {
	if cfg != nil && cfg.invoker != nil {
		return cfg.invoker
	}
	if ctx != nil {
		if invoker, ok := ctx.Value(aiJSInvokerContextKey{}).(AIJSInvoker); ok && invoker != nil {
			return invoker
		}
	}
	return AIJSInvoker(invokeLiteForgeForPathsFunc)
}

func invokeAIJSWithBudget(ctx context.Context, cfg *AIJSExtractConfig, payload string, onPath func(string)) bool {
	if cfg == nil || strings.TrimSpace(payload) == "" || onPath == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return false
	default:
	}

	if cfg.runtimeBudget == nil {
		cfg.runtimeBudget = newAIJSCallBudget(cfg.MaxAIRequests)
	}
	if !cfg.runtimeBudget.take() {
		return false
	}

	callCtx := ctx
	cancel := func() {}
	if cfg.CallTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, cfg.CallTimeout)
	}
	defer cancel()
	_ = configuredAIJSInvoker(callCtx, cfg)(callCtx, cfg, payload, onPath)
	return true
}

// RunAIJSExtractAssets analyzes assets sequentially while sharing one atomic AI
// call budget. Deterministic extraction still runs for every asset, even after
// the AI budget is exhausted.
func RunAIJSExtractAssets(ctx context.Context, assets []AIJSAsset, cfg *AIJSExtractConfig, onPath func(string)) error {
	if onPath == nil {
		return errors.New("ai js extract: onPath is nil")
	}
	if cfg == nil {
		cfg = NewAIJSExtractConfig()
	}
	base := *cfg
	if base.runtimeBudget == nil {
		base.runtimeBudget = newAIJSCallBudget(base.MaxAIRequests)
	}
	for _, asset := range assets {
		if strings.TrimSpace(asset.Body) == "" {
			continue
		}
		assetCfg := base
		assetCfg.assetSourceURL = asset.SourceURL
		assetCfg.assetContentType = asset.ContentType
		if err := runAIJSExtract(ctx, asset.Body, &assetCfg, onPath); err != nil {
			return err
		}
	}
	return nil
}

// withAIJSInvoker is intentionally package-private. Core tests use it for a
// per-config mock; AID integration tests use WithAIJSInvokerContext instead.
func withAIJSInvoker(invoker AIJSInvoker) AIJSExtractOption {
	return func(cfg *AIJSExtractConfig) {
		cfg.invoker = invoker
	}
}
