package aicommon

import (
	"strings"
)

// NormalizeConcreteEvidenceMarkdown normalizes evidence markdown for reuse.
// It keeps evidence optional and preserves the author's wording instead of
// rejecting content via keyword matching.
func NormalizeConcreteEvidenceMarkdown(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.TrimSpace(content)
}