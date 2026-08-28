package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

// Patch apply spin-guard: after consecutive Apply Patch failures, force the model
// onto line-range / old_snippet (same idea as empty-snippet max retry), instead of
// endlessly regenerating brittle unify-diffs.
const (
	maxConsecutivePatchApplyFail = 3

	loopVarPatchApplyFailCountSuffix = "_patch_apply_fail_count"
	loopVarPatchFallbackModeSuffix   = "_patch_fallback_mode"
)

func patchApplyFailCountKey(actionName string) string {
	return actionName + loopVarPatchApplyFailCountSuffix
}

func patchFallbackModeKey(actionName string) string {
	return actionName + loopVarPatchFallbackModeSuffix
}

// IsPatchFallbackMode reports whether consecutive patch failures have forced
// line-range / old_snippet as the preferred modify path.
func IsPatchFallbackMode(loop interface{ Get(string) string }, actionName string) bool {
	if loop == nil || actionName == "" {
		return false
	}
	return loop.Get(patchFallbackModeKey(actionName)) == "true"
}

// IsModifyCodePatchFallbackMode is the yaklang-code loop convenience check
// (action name is fixed to modify_code).
func IsModifyCodePatchFallbackMode(loop interface{ Get(string) string }) bool {
	return IsPatchFallbackMode(loop, "modify_code")
}

func bumpPatchApplyFail(loop *reactloops.ReActLoop, actionName string) (failCount int, fallback bool) {
	if loop == nil {
		return 0, false
	}
	failCount = loop.GetInt(patchApplyFailCountKey(actionName)) + 1
	loop.Set(patchApplyFailCountKey(actionName), fmt.Sprint(failCount))
	if failCount >= maxConsecutivePatchApplyFail {
		loop.Set(patchFallbackModeKey(actionName), "true")
		fallback = true
	}
	return failCount, fallback
}

func resetPatchApplyFail(loop *reactloops.ReActLoop, actionName string) {
	if loop == nil || actionName == "" {
		return
	}
	loop.Set(patchApplyFailCountKey(actionName), "0")
	loop.Set(patchFallbackModeKey(actionName), "false")
}

func buildPatchApplyFailFeedback(applyErr error, failCount int, fallback bool, fullCode, oldPreview string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("【modify_code 失败】Patch 应用失败（文件未改动，连续第 %d/%d 次）: %v\n\n",
		failCount, maxConsecutivePatchApplyFail, applyErr))

	if hint := nearestCodeContextHint(fullCode, oldPreview); hint != "" {
		b.WriteString(hint)
		b.WriteString("\n\n")
	}

	if fallback {
		b.WriteString(`【强制降级】Apply Patch 已连续失败，禁止继续空转同一策略。
下一轮必须改用行号模式或 old_snippet（二选一）：

1) 行号模式（推荐）：
{"@action":"modify_code","modify_start_line":<起始>,"modify_end_line":<结束>,"modify_code_reason":"..."}
<|GEN_CODE_<nonce>|>
...仅替换该行号范围内的新代码（可含完整函数体）...
<|GEN_CODE_END_<nonce>|>

2) old_snippet 精确替换：
{"@action":"modify_code","old_snippet":"<从 CURRENT_CODE 复制的短片段>","modify_code_reason":"..."}
<|GEN_CODE_<nonce>|>
...替换后的新片段...
<|GEN_CODE_END_<nonce>|>

行号请对照上方 CURRENT_CODE；不要再输出 *** Begin Patch，除非你能保证 context/- 行与 CURRENT_CODE 逐字符一致。`)
		return b.String()
	}

	b.WriteString(`请基于本轮 CURRENT_CODE 重新生成 Patch：context 与 '-' 行必须从最新代码【逐字符】复制，不要复用 rebase/上一轮修改前的旧片段。
字符串里的 \n/\r\n 禁止展开成真实换行，也禁止再加一层反斜杠写成 \\n。
系统已尝试兼容 CRLF/LF、行尾空格，以及一次 \\n→\n 过转义纠正；若仍失败，请加长 @@ 上下文确保唯一匹配。

若再失败将被强制降级到 modify_start_line/modify_end_line 或 old_snippet。`)
	return b.String()
}

func buildPatchParseFailFeedback(parseErr error, failCount int, fallback bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("【modify_code 失败】Patch 解析失败（连续第 %d/%d 次）: %v\n\n",
		failCount, maxConsecutivePatchApplyFail, parseErr))
	if fallback {
		b.WriteString(`【强制降级】请改用 modify_start_line/modify_end_line 或 old_snippet，不要继续输出损坏的 *** Begin Patch。`)
		return b.String()
	}
	b.WriteString("请输出完整的 *** Begin Patch ... *** End Patch；若再次失败将强制改用行号/old_snippet 模式。")
	return b.String()
}

// nearestCodeContextHint finds a CURRENT_CODE line that overlaps the failed
// old-text preview, so the model can re-anchor instead of blind-retrying.
func nearestCodeContextHint(fullCode, oldPreview string) string {
	oldPreview = strings.TrimSpace(oldPreview)
	if fullCode == "" || oldPreview == "" {
		return ""
	}
	var needle string
	for _, line := range strings.Split(strings.ReplaceAll(oldPreview, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "@@") {
			continue
		}
		// Strip common patch prefixes if preview still has them.
		if len(line) > 1 && (line[0] == '-' || line[0] == '+' || line[0] == ' ') {
			line = strings.TrimSpace(line[1:])
		}
		if len(line) >= 8 {
			needle = line
			break
		}
	}
	if needle == "" {
		return ""
	}

	normFull := strings.ReplaceAll(fullCode, "\r\n", "\n")
	lines := strings.Split(normFull, "\n")
	matchIdx := -1
	for i, line := range lines {
		if codeLineLooseMatch(line, needle) {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return fmt.Sprintf("CURRENT_CODE 中未找到预览关键行（needle=%s）；请直接用行号模式按 CURRENT_CODE 行号替换。",
			utils.ShrinkTextBlock(needle, 80))
	}

	start := matchIdx - 1
	if start < 0 {
		start = 0
	}
	end := matchIdx + 2
	if end > len(lines) {
		end = len(lines)
	}
	var ctx strings.Builder
	for j := start; j < end; j++ {
		ctx.WriteString(fmt.Sprintf("%d| %s\n", j+1, lines[j]))
	}
	return fmt.Sprintf("CURRENT_CODE 近似锚点（围绕匹配行 %d）：\n%s", matchIdx+1, strings.TrimRight(ctx.String(), "\n"))
}

func codeLineLooseMatch(haystackLine, needle string) bool {
	h := strings.TrimSpace(haystackLine)
	n := strings.TrimSpace(needle)
	if h == "" || n == "" {
		return false
	}
	if strings.Contains(h, n) || strings.Contains(n, h) {
		return true
	}
	// Go-style typed signatures often drift from Yak DSL lines; match on leading ident.
	token := leadingCodeIdent(n)
	return len(token) >= 6 && strings.Contains(h, token)
}

func leadingCodeIdent(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		break
	}
	return b.String()
}
