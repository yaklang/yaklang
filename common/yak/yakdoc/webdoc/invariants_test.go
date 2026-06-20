package webdoc

import (
	"strings"
	"testing"
)

// 关键词: CheckMarkdownInvariants 测试, 失败模式守卫

func TestCheckMarkdownInvariantsValid(t *testing.T) {
	valid := []string{
		// 链接锚点齐全、行内代码里的 < 不算违例、围栏成对
		"# t {#library-t}\n\n[x](#foo)\n\n## 函数详情\n\n### foo {#foo}\n\n```go\nfoo()\n```\n\ndesc with `a<b` inline\n\n---\n",
		// markdown 链接里的 http:// 允许保留(可点击)
		"# t {#library-t}\n\nsee [site](http://example.com)\n",
		// 表格列一致，含行内代码管道
		"# t {#library-t}\n\n|a|b|\n|:--|:--|\n| `x|y` | z |\n",
	}
	for i, md := range valid {
		if err := CheckMarkdownInvariants(md); err != nil {
			t.Fatalf("valid case %d should pass, got: %v\n%s", i, err, md)
		}
	}
}

func TestCheckMarkdownInvariantsCatches(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{"duplicate id", "# a {#x}\n\n### b {#x}\n", "duplicate heading id"},
		{"unbalanced fence", "# a {#a}\n\n```go\ncode\n", "unbalanced code fence"},
		{"table mismatch", "# t {#t}\n\n|a|b|\n|:--|:--|\n|1|2|3|\n", "table column mismatch"},
		{"bare url", "# t {#t}\n\nvisit http://x now\n", "bare URL not neutralized"},
		{"raw lt", "# t {#t}\n\na < b here\n", "raw '<'"},
		{"multiple h1", "# a {#a}\n\n# b {#b}\n", "expected exactly 1 H1"},
		{"zero h1", "## a {#a}\n", "expected exactly 1 H1"},
		{"deep heading", "# a {#a}\n\n#### deep {#d}\n", "exceeds 3"},
		{"missing anchor", "# t {#t}\n\n[x](#missing)\n", "no matching heading id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckMarkdownInvariants(c.md)
			if err == nil {
				t.Fatalf("expected violation %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected error containing %q, got: %v", c.want, err)
			}
		})
	}
}
