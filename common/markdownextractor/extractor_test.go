package markdownextractor

import (
	"fmt"
	"testing"
)

func TestExtractMarkdownCode(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expect   []struct {
			typeName string
			code     string
		}
	}{
		{
			name:     "basic code block",
			markdown: "# Title\n\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "go",
					code:     "func main() {\n    fmt.Println(\"Hello\")\n}",
				},
			},
		},
		{
			name:     "multiple code blocks",
			markdown: "# Multiple Code Blocks\n\n```python\nprint(\"Hello\")\n```\n\nSome text\n\n```javascript\nconsole.log(\"World\")\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "python",
					code:     "print(\"Hello\")",
				},
				{
					typeName: "javascript",
					code:     "console.log(\"World\")",
				},
			},
		},
		{
			name:     "code block with empty type",
			markdown: "```\nplain text\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "",
					code:     "plain text",
				},
			},
		},
		{
			name:     "nested backticks",
			markdown: "```markdown\n`code` inside code block\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "markdown",
					code:     "`code` inside code block\n",
				},
			},
		},
		{
			name:     "code block with spaces",
			markdown: "```  go  \nfunc main() {}\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "go",
					code:     "func main() {}",
				},
			},
		},
		{
			name:     "code block with special characters",
			markdown: "```go\nfunc main() {\n    // 中文注释\n    fmt.Println(\"特殊字符: ~!@#$%^&*()_+\")\n}\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "go",
					code:     "func main() {\n    // 中文注释\n    fmt.Println(\"特殊字符: ~!@#$%^&*()_+\")\n}",
				},
			},
		},
		{
			name:     "code block with line breaks in type",
			markdown: "```go\npython\nfunc main() {}\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "go",
					code:     "python\nfunc main() {}",
				},
			},
		},
		{
			name:     "empty code block",
			markdown: "```go\n\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "go",
					code:     "",
				},
			},
		},
		{
			name:     "code block with only spaces",
			markdown: "```go\n    \n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "go",
					code:     "    ",
				},
			},
		},
		{
			name:     "code block with backticks in content",
			markdown: "```go\nfmt.Println(\"```\")\n```",
			expect: []struct {
				typeName string
				code     string
			}{
				{
					typeName: "go",
					code:     "fmt.Println(\"```\")",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results []struct {
				typeName string
				code     string
			}

			fmt.Println(tt.markdown)
			_, err := ExtractMarkdownCode(tt.markdown, func(typeName string, code string, startOffset int, endOffset int) {
				results = append(results, struct {
					typeName string
					code     string
				}{
					typeName: typeName,
					code:     code,
				})
			})

			if err != nil {
				t.Fatalf("ExtractMarkdownCode() error = %v", err)
			}

			if len(results) != len(tt.expect) {
				t.Fatalf("expected %d code blocks, got %d", len(tt.expect), len(results))
			}

			for i, result := range results {
				if result.typeName != tt.expect[i].typeName {
					t.Errorf("code block %d: expected type %q, got %q", i, tt.expect[i].typeName, result.typeName)
				}
				if result.code != tt.expect[i].code {
					t.Errorf("code block %d: expected code %q, got %q", i, tt.expect[i].code, result.code)
				}
			}
		})
	}
}
