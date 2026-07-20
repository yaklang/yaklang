package loop_yaklangcode

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

const (
	autoRecoveryMaxPatterns    = 2
	autoRecoveryMaxHitsEach    = 3
	autoRecoveryContextLines   = 8
	autoRecoverySampleMaxBytes = 3072
)

// recoverySuggestion is a concrete mid-loop tool call the AI should make after a lint error.
type recoverySuggestion struct {
	Kind    string // "grep" | "yakdoc_function" | "yakdoc_search" | "yakdoc_overview"
	Pattern string // grep pattern or yakdoc search keyword
	Library string // yakdoc library name
	Func    string // yakdoc function name
	Reason  string
}

var (
	reValueUndefined     = regexp.MustCompile(`(?i)Value undefined:\s*([A-Za-z_][\w.]*)`)
	reExternLibMissing   = regexp.MustCompile(`(?i)ExternLib\s*\[([^\]]+)\]\s*don't has\s*\[([^\]]+)\]`)
	reCantFindDefinition = regexp.MustCompile(`(?i)Can't find definition of this variable[:\s]*([A-Za-z_][\w.]*)`)
	reIdentDotIdent      = regexp.MustCompile(`\b([A-Za-z_][\w]*)\.([A-Za-z_][\w]*)\b`)
	reBareIdent          = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]{2,})\b`)
	reErrLineRange       = regexp.MustCompile(`in \[(\d+):`)
)

// deriveRecoverySuggestions extracts actionable grep/yakdoc follow-ups from a compiler error.
func deriveRecoverySuggestions(normalizedMessage, lineContent string) []recoverySuggestion {
	msg := strings.TrimSpace(normalizedMessage)
	line := strings.TrimSpace(lineContent)
	var out []recoverySuggestion

	if m := reExternLibMissing.FindStringSubmatch(msg); len(m) == 3 {
		lib, member := strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
		out = append(out,
			recoverySuggestion{
				Kind: "yakdoc_function", Library: lib, Func: member,
				Reason: fmt.Sprintf("确认 %s.%s 是否存在及正确签名", lib, member),
			},
			recoverySuggestion{
				Kind: "grep", Pattern: regexp.QuoteMeta(lib+"."+member) + "|" + regexp.QuoteMeta(lib) + `\.`,
				Reason: "检索该库真实用法样例",
			},
			recoverySuggestion{
				Kind: "yakdoc_overview", Library: lib,
				Reason: "查看库可用 API 列表",
			},
		)
		return dedupeRecoverySuggestions(out)
	}

	if m := reValueUndefined.FindStringSubmatch(msg); len(m) == 2 {
		name := strings.TrimSpace(m[1])
		out = append(out, recoverySuggestion{
			Kind: "grep", Pattern: regexp.QuoteMeta(name),
			Reason: "查找符号 " + name + " 的正确写法",
		})
		if strings.Contains(name, ".") {
			parts := strings.SplitN(name, ".", 2)
			out = append(out, recoverySuggestion{
				Kind: "yakdoc_function", Library: parts[0], Func: parts[1],
				Reason: "查权威签名",
			})
		} else {
			out = append(out, recoverySuggestion{
				Kind: "yakdoc_search", Pattern: name,
				Reason: "按功能搜索相近 API",
			})
		}
		return dedupeRecoverySuggestions(out)
	}

	if m := reCantFindDefinition.FindStringSubmatch(msg); len(m) == 2 {
		name := strings.TrimSpace(m[1])
		out = append(out, recoverySuggestion{
			Kind: "grep", Pattern: regexp.QuoteMeta(name),
			Reason: "查找变量/函数定义样例",
		})
	}

	// Go-style var (...) block — Yaklang DSL does not support this.
	if strings.Contains(msg, "no viable alternative") || strings.Contains(msg, "Syntax Error") || strings.Contains(msg, "基础语法错误") {
		if strings.Contains(line, "var (") || strings.Contains(line, "var(") ||
			strings.Contains(msg, "var(\\n") || strings.Contains(msg, "var(\n") {
			out = append(out,
				recoverySuggestion{
					Kind: "grep", Pattern: `sync\.NewMap\s*\(`,
					Reason: "Yaklang 不用 Go 的 var(...) 块；查 sync.NewMap 等声明样例",
				},
				recoverySuggestion{
					Kind: "grep", Pattern: `=\s*sync\.NewMap`,
					Reason: "查顶层直接赋值声明写法",
				},
			)
		}
		if strings.Contains(line, "assert") && (strings.Contains(line, "assertc") || strings.Contains(msg, "assertc")) {
			out = append(out, recoverySuggestion{
				Kind: "grep", Pattern: `assert\.(Equal|true|nil)`,
				Reason: "查 assert 正确用法（非 assertc）",
			})
		}
		if strings.Contains(msg, "mismatched input") || strings.Contains(msg, "expecting") {
			out = append(out, recoverySuggestion{
				Kind: "grep", Pattern: `func\s+\w+\s*\(`,
				Reason: "对照完整函数/语句写法",
			})
		}
	}

	// Member / call access failures: derive lib.func from line.
	if strings.Contains(msg, "unable to access") || strings.Contains(msg, "don't has") ||
		strings.Contains(msg, "Invalid operation") {
		if pairs := reIdentDotIdent.FindAllStringSubmatch(line, 3); len(pairs) > 0 {
			for _, p := range pairs {
				lib, member := p[1], p[2]
				if isYaklangNoiseIdent(lib) {
					continue
				}
				out = append(out,
					recoverySuggestion{
						Kind: "yakdoc_function", Library: lib, Func: member,
						Reason: fmt.Sprintf("查 %s.%s 签名", lib, member),
					},
					recoverySuggestion{
						Kind: "grep", Pattern: regexp.QuoteMeta(lib+"."+member),
						Reason: "查该 API 用法样例",
					},
				)
			}
		}
	}

	// Argument type / count errors: prefer yakdoc on callee.
	if strings.Contains(msg, "cannot use as") || strings.Contains(msg, "Not enough arguments") ||
		strings.Contains(msg, "argument (") {
		if pairs := reIdentDotIdent.FindAllStringSubmatch(line, 2); len(pairs) > 0 {
			lib, member := pairs[0][1], pairs[0][2]
			out = append(out, recoverySuggestion{
				Kind: "yakdoc_function", Library: lib, Func: member,
				Reason: "参数类型/个数以 yakdoc 签名为准，禁止换函数名硬猜",
			})
		}
	}

	// Generic fallback from line identifiers / dotted calls.
	if len(out) == 0 && line != "" {
		if pairs := reIdentDotIdent.FindAllStringSubmatch(line, 2); len(pairs) > 0 {
			lib, member := pairs[0][1], pairs[0][2]
			if !isYaklangNoiseIdent(lib) {
				out = append(out,
					recoverySuggestion{
						Kind: "grep", Pattern: regexp.QuoteMeta(lib+"."+member) + "|" + regexp.QuoteMeta(lib) + `\.`,
						Reason: "按行内 API 检索正确写法",
					},
					recoverySuggestion{
						Kind: "yakdoc_function", Library: lib, Func: member,
						Reason: "确认 API 是否存在",
					},
				)
			}
		} else if ids := reBareIdent.FindAllString(line, 6); len(ids) > 0 {
			for _, id := range ids {
				if isYaklangNoiseIdent(id) {
					continue
				}
				out = append(out, recoverySuggestion{
					Kind: "grep", Pattern: regexp.QuoteMeta(id),
					Reason: "按标识符检索样例",
				})
				break
			}
		}
	}

	if len(out) == 0 && (strings.Contains(msg, "no viable alternative") || strings.Contains(msg, "Syntax Error") || strings.Contains(msg, "基础语法错误")) {
		out = append(out, recoverySuggestion{
			Kind: "grep", Pattern: `yakit\.AutoInitYakit|YAK_MAIN`,
			Reason: "语法错误且无法提取标识符：先检索标准脚本骨架/声明写法",
		})
	}

	return dedupeRecoverySuggestions(out)
}

func isYaklangNoiseIdent(s string) bool {
	switch strings.ToLower(s) {
	case "if", "for", "func", "return", "true", "false", "nil", "var", "const",
		"type", "map", "string", "int", "bool", "byte", "error", "go", "defer",
		"range", "else", "break", "continue", "switch", "case", "default",
		"log", "fmt", "make", "len", "append", "println", "print":
		return true
	default:
		return len(s) < 2
	}
}

func dedupeRecoverySuggestions(in []recoverySuggestion) []recoverySuggestion {
	seen := make(map[string]struct{}, len(in))
	out := make([]recoverySuggestion, 0, len(in))
	for _, s := range in {
		key := s.Kind + "|" + s.Pattern + "|" + s.Library + "|" + s.Func
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

// formatRecoveryNextStepBlock renders a forced next-step recipe for AI Feedback.
func formatRecoveryNextStepBlock(suggestions []recoverySuggestion) string {
	if len(suggestions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n【下一步·强制】收到本反馈后，下一动作必须先执行下列检索之一，再基于结果做一次 modify_code；禁止继续盲目 patch。\n")
	maxShow := 3
	if len(suggestions) < maxShow {
		maxShow = len(suggestions)
	}
	for i := 0; i < maxShow; i++ {
		s := suggestions[i]
		switch s.Kind {
		case "grep":
			b.WriteString(fmt.Sprintf("%d. grep_yaklang_samples — %s\n", i+1, s.Reason))
			b.WriteString(fmt.Sprintf("   {\"@action\":\"grep_yaklang_samples\",\"pattern\":%q,\"context_lines\":20}\n", s.Pattern))
		case "yakdoc_function":
			b.WriteString(fmt.Sprintf("%d. yakdoc_function_details — %s\n", i+1, s.Reason))
			b.WriteString(fmt.Sprintf("   {\"@action\":\"yakdoc_function_details\",\"library\":%q,\"function\":[%q]}\n", s.Library, s.Func))
		case "yakdoc_search":
			b.WriteString(fmt.Sprintf("%d. yakdoc_search — %s\n", i+1, s.Reason))
			b.WriteString(fmt.Sprintf("   {\"@action\":\"yakdoc_search\",\"query\":%q}\n", s.Pattern))
		case "yakdoc_overview":
			b.WriteString(fmt.Sprintf("%d. yakdoc_module_overview — %s\n", i+1, s.Reason))
			b.WriteString(fmt.Sprintf("   {\"@action\":\"yakdoc_module_overview\",\"library\":%q}\n", s.Library))
		}
	}
	b.WriteString("说明：lint 失败时 Init 已覆盖的 pattern 也可再次 grep；检索结果会出现在下一轮 Feedback/时间线。\n")
	return b.String()
}

// buildActionableRecoveryHint returns AI-facing next-step text for one lint error.
func buildActionableRecoveryHint(normalizedMessage, lineContent string) string {
	return formatRecoveryNextStepBlock(deriveRecoverySuggestions(normalizedMessage, lineContent))
}

// autoGrepSamplesForLintErrors runs a lightweight AIKB grep for derived patterns and
// appends top hits into Feedback so the model sees real samples without an extra turn.
func autoGrepSamplesForLintErrors(searcher *ziputil.ZipGrepSearcher, errMsg, code string) string {
	if searcher == nil || strings.TrimSpace(errMsg) == "" {
		return ""
	}
	patterns := collectGrepPatternsFromErrMsg(errMsg, code)
	if len(patterns) == 0 {
		return ""
	}
	if len(patterns) > autoRecoveryMaxPatterns {
		patterns = patterns[:autoRecoveryMaxPatterns]
	}

	grepOpts := []ziputil.GrepOption{
		ziputil.WithGrepCaseSensitive(false),
		ziputil.WithContext(autoRecoveryContextLines),
	}

	var blocks []string
	for _, pattern := range patterns {
		results, err := searcher.GrepRegexp(pattern, grepOpts...)
		if err != nil || len(results) == 0 {
			results, err = searcher.GrepSubString(pattern, grepOpts...)
		}
		if err != nil || len(results) == 0 {
			log.Infof("auto recovery grep: no hits for pattern %q", pattern)
			continue
		}
		hits := GrepResultsToSampleHits(pattern, results, autoRecoveryMaxHitsEach)
		var b strings.Builder
		b.WriteString(fmt.Sprintf("### pattern=%q (%d hits, showing %d)\n", pattern, len(results), len(hits)))
		for _, h := range hits {
			b.WriteString(fmt.Sprintf("- %s:%d\n", h.FileName, h.Line))
			b.WriteString(utils.ShrinkTextBlock(h.Content, 600))
			b.WriteString("\n")
		}
		blocks = append(blocks, b.String())
	}
	if len(blocks) == 0 {
		return ""
	}

	body := utils.ShrinkTextBlock(strings.Join(blocks, "\n"), autoRecoverySampleMaxBytes)
	return "\n【自动检索样例·系统已代查 AIKB】下列片段按当前语法错误自动 grep，请对照改写；若仍不够，用【下一步·强制】中的 pattern 再调 grep_yaklang_samples / yakdoc。\n" + body
}

func collectGrepPatternsFromErrMsg(errMsg, code string) []string {
	seen := make(map[string]struct{})
	var patterns []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		patterns = append(patterns, p)
	}

	for _, line := range strings.Split(errMsg, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[Error]") && !strings.HasPrefix(trimmed, "[Warning]") {
			continue
		}
		core := extractCoreCompilerMessage(trimmed)
		codeLine := guessCodeLineFromErrLine(trimmed, code)
		for _, s := range deriveRecoverySuggestions(core, codeLine) {
			if s.Kind == "grep" && s.Pattern != "" {
				add(s.Pattern)
			}
		}
	}
	return patterns
}

func guessCodeLineFromErrLine(errLine, code string) string {
	m := reErrLineRange.FindStringSubmatch(errLine)
	if len(m) < 2 || code == "" {
		return ""
	}
	lineNo := 0
	fmt.Sscanf(m[1], "%d", &lineNo)
	if lineNo <= 0 {
		return ""
	}
	lines := strings.Split(code, "\n")
	if lineNo > len(lines) {
		return ""
	}
	return lines[lineNo-1]
}
