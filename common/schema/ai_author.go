package schema

import "strings"

const (
	AIResourceAuthorAnonymous = "anonymous"
	AIResourceAuthorBuiltin   = "yaklang.io"
)

func NormalizeAIResourceAuthor(author string, fallback string) string {
	if strings.TrimSpace(author) == "" {
		return fallback
	}
	return author
}
