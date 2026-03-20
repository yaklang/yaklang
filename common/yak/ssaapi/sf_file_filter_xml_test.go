package ssaapi

import "testing"

func TestIsYAMLStructuredRootOnly(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "plain text scalar",
			content: "hello world",
			want:    false,
		},
		{
			name:    "java source like text",
			content: "public class App { void f() {} }",
			want:    false,
		},
		{
			name:    "multiline plain text",
			content: "line1\nline2\nline3\n",
			want:    false,
		},
		{
			name:    "yaml map",
			content: "spring:\n  datasource:\n    password: secret\n",
			want:    true,
		},
		{
			name:    "yaml list",
			content: "- a\n- b\n",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isYAML([]byte(tt.content)); got != tt.want {
				t.Fatalf("isYAML(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestCheckFileContentTypeStructuredOnly(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    FileFilterXpathKind
	}{
		{
			name:    "plain text should be invalid",
			content: "hello world",
			want:    FileFilterXPathUnValid,
		},
		{
			name:    "source code should be invalid",
			content: "package main;\nfunc main(){println(1)}",
			want:    FileFilterXPathUnValid,
		},
		{
			name:    "json object should stay json",
			content: "{\"password\":\"secret\"}",
			want:    FileFilterXPathJson,
		},
		{
			name:    "yaml map should be yaml",
			content: "password: secret\n",
			want:    FileFilterXPathYaml,
		},
		{
			name:    "yaml list should be yaml",
			content: "- secret\n- token\n",
			want:    FileFilterXPathYaml,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkFileContentType([]byte(tt.content)); got != tt.want {
				t.Fatalf("checkFileContentType(%q) = %s, want %s", tt.content, got, tt.want)
			}
		})
	}
}
