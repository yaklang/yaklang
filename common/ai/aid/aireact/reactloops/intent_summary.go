package reactloops

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const IntentSummaryRecommendedRunes = 24

func CompactIntentSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	summary = strings.Join(strings.Fields(strings.ReplaceAll(summary, "\u3000", " ")), " ")

	for _, marker := range []string{"目的是：", "目的：", "意图：", "核心意图："} {
		if idx := strings.Index(summary, marker); idx >= 0 {
			summary = summary[idx+len(marker):]
			break
		}
	}

	for _, marker := range []string{"。", "；", ";", "，推荐", ", recommend", "通过搜索", "推荐使用", "推荐能力", "推荐工具", "蓝图", "技能", "专注模式"} {
		if idx := strings.Index(summary, marker); idx >= 0 {
			summary = summary[:idx]
			break
		}
	}

	summary = strings.Trim(summary, " ，。；;:：\"'“”‘’[]【】()（）")
	if summary == "" {
		return ""
	}

	return strings.TrimSpace(summary)
}

func CompactCapabilityNames(names string, maxItems int) string {
	if maxItems <= 0 {
		maxItems = 3
	}
	clean := NormalizeCapabilityNames(names)
	if len(clean) == 0 {
		return ""
	}
	if len(clean) <= maxItems {
		return strings.Join(clean, "; ")
	}
	return strings.Join(clean[:maxItems], "; ") + " ..."
}

func NormalizeCapabilityNames(names string) []string {
	return normalizeCapabilityNames(names)
}

func normalizeCapabilityNames(names string) []string {
	names = strings.TrimSpace(names)
	if names == "" || names == "[]" {
		return nil
	}

	if unquoted, ok := tryUnquoteJSONString(names); ok {
		names = strings.TrimSpace(unquoted)
		if names == "" || names == "[]" {
			return nil
		}
	}

	var items []string
	if strings.HasPrefix(names, "[") {
		if parsed, ok := tryParseJSONArray(names); ok {
			items = parsed
		} else {
			items = splitCapabilityNames(names)
		}
	} else {
		items = splitCapabilityNames(names)
	}

	clean := make([]string, 0, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || item == "[]" {
			continue
		}
		if unquoted, ok := tryUnquoteJSONString(item); ok {
			item = strings.TrimSpace(unquoted)
		}
		for _, part := range splitCapabilityNames(item) {
			part = strings.TrimSpace(strings.Trim(part, "[]\"'"))
			if part == "" {
				continue
			}
			upper := strings.ToUpper(part)
			if upper == "__DEFAULT__" || upper == "DEFAULT" {
				continue
			}
			if _, exists := seen[part]; exists {
				continue
			}
			seen[part] = struct{}{}
			clean = append(clean, part)
		}
	}
	return clean
}

func tryUnquoteJSONString(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	unquoted, err := strconv.Unquote(raw)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

func tryParseJSONArray(raw string) ([]string, bool) {
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		switch v := item.(type) {
		case string:
			out = append(out, v)
		default:
			out = append(out, strings.TrimSpace(fmt.Sprint(v)))
		}
	}
	return out, true
}

func splitCapabilityNames(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';' || r == '，' || r == '；'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
