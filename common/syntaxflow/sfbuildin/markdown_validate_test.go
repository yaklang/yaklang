package sfbuildin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateMarkdownText(t *testing.T) {
	t.Run("valid multiline markdown", func(t *testing.T) {
		content := "### Title\n\nSome text.\n\n```go\nfmt.Println(\"\\n\")\n```\n"
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("literal newline escape in prose", func(t *testing.T) {
		content := "### Title\\n\\nSome text."
		issues := validateMarkdownText(content)
		require.Len(t, issues, 1)
		require.Contains(t, issues[0], "literal \\n")
	})

	t.Run("inline code fence", func(t *testing.T) {
		content := "see example: ```go\ncode\n```"
		issues := validateMarkdownText(content)
		require.NotEmpty(t, issues)
		require.Contains(t, issues[0], "code fence")
	})

	t.Run("inline code escape notation is allowed", func(t *testing.T) {
		content := "Remove `\\n` and `\\r` from log input."
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("quoted escape notation is allowed", func(t *testing.T) {
		content := "Filter \"\\n\" and '\\n' from log input."
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("crlf notation in prose is allowed", func(t *testing.T) {
		content := "Header splitting via `\\r\\n` injection."
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("code fence with language on own line", func(t *testing.T) {
		content := "```java\nclass A {}\n```"
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("broken windows path separator check", func(t *testing.T) {
		content := "### fix\n```java\nif (name.startsWith(\"\\\"\")) {\n}\n```\n"
		issues := validateMarkdownText(content)
		require.NotEmpty(t, issues)
		require.Contains(t, issues[0], "corrupted path separator")
	})

	t.Run("valid windows path separator check", func(t *testing.T) {
		content := "### fix\n```java\nif (name.startsWith(\"\\\\\")) {\n}\n```\n"
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("json escaped import in go code block", func(t *testing.T) {
		content := "### desc\n```go\nimport (\n\\\"fmt\\\"\n)\n```\n"
		issues := validateMarkdownText(content)
		require.NotEmpty(t, issues)
		require.Contains(t, issues[0], "JSON-style escaped quote")
	})

	t.Run("json escaped call in java code block", func(t *testing.T) {
		content := "### desc\n```java\nString v = request.getParameter(\\\"name\\\");\n```\n"
		issues := validateMarkdownText(content)
		require.NotEmpty(t, issues)
		require.Contains(t, issues[0], "JSON-style escaped quote")
	})

	t.Run("json escaped index in php code block", func(t *testing.T) {
		content := "### desc\n```php\n$path = $_GET[\\\"path\\\"];\n```\n"
		issues := validateMarkdownText(content)
		require.NotEmpty(t, issues)
		require.Contains(t, issues[0], "JSON-style escaped quote")
	})

	t.Run("html attribute escapes in c code block are allowed", func(t *testing.T) {
		content := "### fix\n```c\nprintf(\"<a href=\\\"/default-page\\\">link</a>\");\n```\n"
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("embedded json literal in java code block is allowed", func(t *testing.T) {
		content := "### desc\n```java\nString json = \"{\\\"name\\\":\\\"test\\\"}\";\n```\n"
		require.Empty(t, validateMarkdownText(content))
	})

	t.Run("multiline java string with inner escapes is allowed", func(t *testing.T) {
		content := "### desc\n```java\nString xml = \"<tag class=\\\"value\\\">\n<next/>\n\";\n```\n"
		require.Empty(t, validateMarkdownText(content))
	})
}
