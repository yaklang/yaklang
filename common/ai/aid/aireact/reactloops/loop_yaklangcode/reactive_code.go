package loop_yaklangcode

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	reactiveCodeMaxBytes      = 8 * 1024
	reactiveSamplesWhenCoded  = 4 * 1024
	reactiveCodeFocusPadLines = 25
)

var (
	reactiveErrLineRangeRe = regexp.MustCompile(`\[(\d+):\d+\s*--\s*(\d+):\d+\]`)
	reactiveErrLineMarkRe  = regexp.MustCompile(`(?m)^\s*(\d+)\s*>`)
)

// formatReactiveCurrentCode builds numbered CURRENT_CODE for reactive prompts.
// On lint failure, prefer a focus window around error lines to shrink dynamic prompt size.
func formatReactiveCurrentCode(code string, lineBase int, feedback string, lintFailed bool) string {
	code = strings.TrimRight(code, "\r\n")
	if strings.TrimSpace(code) == "" {
		return ""
	}
	startLine := lineBase + 1
	if startLine < 1 {
		startLine = 1
	}
	numbered := utils.PrefixLinesWithLineNumbersFrom(startLine, code)
	if !lintFailed {
		return utils.ShrinkTextBlock(numbered, reactiveCodeMaxBytes)
	}
	focus := extractErrorLineNumbers(feedback)
	if len(focus) == 0 {
		return utils.ShrinkTextBlock(numbered, reactiveCodeMaxBytes)
	}
	return focusNumberedCodeAroundLines(numbered, startLine, focus, reactiveCodeFocusPadLines, reactiveCodeMaxBytes)
}

// shrinkReactiveSamplesAfterCodeExists reduces Init sample re-injection once code exists.
func shrinkReactiveSamplesAfterCodeExists(samples string, hasCode bool) string {
	if !hasCode || strings.TrimSpace(samples) == "" {
		return samples
	}
	return utils.ShrinkTextBlock(samples, reactiveSamplesWhenCoded)
}

func extractErrorLineNumbers(feedback string) []int {
	seen := map[int]struct{}{}
	var out []int
	add := func(n int) {
		if n <= 0 {
			return
		}
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	for _, m := range reactiveErrLineRangeRe.FindAllStringSubmatch(feedback, -1) {
		if len(m) < 3 {
			continue
		}
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		add(a)
		add(b)
	}
	for _, m := range reactiveErrLineMarkRe.FindAllStringSubmatch(feedback, -1) {
		if len(m) < 2 {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		add(n)
	}
	sort.Ints(out)
	return out
}

func focusNumberedCodeAroundLines(numbered string, startLine int, focusLines []int, pad, maxBytes int) string {
	lines := strings.Split(numbered, "\n")
	if len(lines) == 0 {
		return ""
	}
	total := len(lines)
	keep := make([]bool, total)
	for _, abs := range focusLines {
		rel := abs - startLine
		lo := rel - pad
		hi := rel + pad
		if lo < 0 {
			lo = 0
		}
		if hi >= total {
			hi = total - 1
		}
		for i := lo; i <= hi; i++ {
			keep[i] = true
		}
	}
	var b strings.Builder
	omitted := false
	for i, line := range lines {
		if !keep[i] {
			omitted = true
			continue
		}
		if omitted {
			b.WriteString("... (省略未报错区域，完整文件已在编辑器中)\n")
			omitted = false
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	out := strings.TrimRight(b.String(), "\n")
	return utils.ShrinkTextBlock(out, maxBytes)
}
