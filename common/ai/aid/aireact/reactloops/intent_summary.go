package reactloops

import "strings"

const IntentSummaryMaxRunes = 20

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

	runes := []rune(summary)
	if len(runes) > IntentSummaryMaxRunes {
		summary = string(runes[:IntentSummaryMaxRunes])
	}
	return strings.TrimSpace(summary)
}

func CompactCapabilityNames(names string, maxItems int) string {
	if maxItems <= 0 {
		maxItems = 3
	}
	parts := strings.FieldsFunc(names, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';' || r == '，' || r == '；'
	})
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return ""
	}
	if len(clean) <= maxItems {
		return strings.Join(clean, ", ")
	}
	return strings.Join(clean[:maxItems], ", ") + " ..."
}
