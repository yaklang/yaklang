package aicommon

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

var vagueEvidencePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(^|[\s,，、/:：;；()（）\[\]{}])等($|[\s,，、。；;:：])`),
	regexp.MustCompile(`等等`),
	regexp.MustCompile(`其他`),
	regexp.MustCompile(`其它`),
	regexp.MustCompile(`若干`),
	regexp.MustCompile(`etc\.`),
}

// NormalizeConcreteEvidenceMarkdown validates that evidence markdown is concrete enough
// for direct display and long-term accumulation. It rejects vague placeholders such as “等”.
func NormalizeConcreteEvidenceMarkdown(content string) (string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return "", nil
	}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		for _, pattern := range vagueEvidencePatterns {
			if pattern.MatchString(line) {
				return "", utils.Errorf("evidence must enumerate concrete items and must not use vague wording: %s", line)
			}
		}
	}

	return content, nil
}