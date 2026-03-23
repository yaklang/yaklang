package yakit

import (
	"testing"
)

// formatRegexpGroupsTestConfig FormatRegexpGroups 测试配置
type formatRegexpGroupsTestConfig struct {
	name     string
	template string
	groupBy  func(int) string
	expect   string
}

func TestFormatRegexpGroups_Config(t *testing.T) {
	for _, cfg := range []formatRegexpGroupsTestConfig{
		{
			name:     "dollar_syntax",
			template: "$1个$3",
			groupBy: func(n int) string {
				switch n {
				case 1:
					return "a"
				case 3:
					return "c"
				default:
					return ""
				}
			},
			expect: "a个c",
		},
		{
			name:     "backslash_syntax",
			template: `\1-\2`,
			groupBy: func(n int) string {
				switch n {
				case 1:
					return "a"
				case 2:
					return "b"
				default:
					return ""
				}
			},
			expect: "a-b",
		},
		{
			name:     "brace_syntax",
			template: "{1}_{2}",
			groupBy: func(n int) string {
				switch n {
				case 1:
					return "a"
				case 2:
					return "b"
				default:
					return ""
				}
			},
			expect: "a_b",
		},
		{
			name:     "dollar10_before_dollar1",
			template: "$10-$1",
			groupBy: func(n int) string {
				switch n {
				case 1:
					return "one"
				case 10:
					return "ten"
				default:
					return ""
				}
			},
			expect: "ten-one",
		},
		{
			name:     "empty_template",
			template: "",
			groupBy:  func(n int) string { return "x" },
			expect:   "",
		},
		{
			name:     "no_placeholders",
			template: "literal only",
			groupBy:  func(n int) string { return "x" },
			expect:   "literal only",
		},
	} {
		t.Run(cfg.name, func(t *testing.T) {
			out := FormatRegexpGroups(cfg.template, cfg.groupBy)
			if out != cfg.expect {
				t.Fatalf("expected %q, got %q", cfg.expect, out)
			}
		})
	}
}
