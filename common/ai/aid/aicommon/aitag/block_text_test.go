package aitag

import (
	"io"
	"strings"
	"testing"
)

// TestBlockTextFormatting tests that newlines after start tags and before end tags are stripped
func TestBlockTextFormatting(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "inline format",
			input:    `<|CODE_0|>abc<|CODE_END_0|>`,
			expected: "abc",
		},
		{
			name: "block format with newlines",
			input: `<|CODE_0|>
abc
<|CODE_END_0|>`,
			expected: "abc",
		},
		{
			name: "block format with newline after tag only",
			input: `<|CODE_0|>
abc<|CODE_END_0|>`,
			expected: "abc",
		},
		{
			name: "block format with newline before end tag only",
			input: `<|CODE_0|>abc
<|CODE_END_0|>`,
			expected: "abc",
		},
		{
			name: "multi-line content with block format",
			input: `<|CODE_0|>
line1
line2
line3
<|CODE_END_0|>`,
			expected: "line1\nline2\nline3",
		},
		{
			name: "content with intentional newlines",
			input: `<|CODE_0|>

content

<|CODE_END_0|>`,
			expected: "\ncontent\n",
		},
		{
			name:     "empty content inline",
			input:    `<|CODE_0|><|CODE_END_0|>`,
			expected: "",
		},
		{
			name: "empty content block",
			input: `<|CODE_0|>
<|CODE_END_0|>`,
			expected: "",
		},
		{
			name: "single newline content",
			input: `<|CODE_0|>

<|CODE_END_0|>`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedContent string
			err := Parse(strings.NewReader(tt.input), WithCallback("CODE", "0", func(reader io.Reader) {
				content, _ := io.ReadAll(reader)
				capturedContent = string(content)
			}))

			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if capturedContent != tt.expected {
				t.Errorf("Expected content: %q\nGot: %q", tt.expected, capturedContent)
			}
		})
	}
}

// TestBlockTextWithComplexContent tests block formatting with more complex content
func TestBlockTextWithComplexContent(t *testing.T) {
	input := `<|CODE_go123|>
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
<|CODE_END_go123|>`

	expectedContent := `package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}`

	var capturedContent string
	err := Parse(strings.NewReader(input), WithCallback("CODE", "go123", func(reader io.Reader) {
		content, _ := io.ReadAll(reader)
		capturedContent = string(content)
	}))

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if capturedContent != expectedContent {
		t.Errorf("Expected content:\n%s\n\nGot:\n%s", expectedContent, capturedContent)
	}
}

// TestBlockTextPreservesInternalNewlines ensures internal newlines are preserved
func TestBlockTextPreservesInternalNewlines(t *testing.T) {
	input := `<|TEXT_test|>
First paragraph.

Second paragraph.

Third paragraph.
<|TEXT_END_test|>`

	expectedContent := `First paragraph.

Second paragraph.

Third paragraph.`

	var capturedContent string
	err := Parse(strings.NewReader(input), WithCallback("TEXT", "test", func(reader io.Reader) {
		content, _ := io.ReadAll(reader)
		capturedContent = string(content)
	}))

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if capturedContent != expectedContent {
		t.Errorf("Expected content:\n%s\n\nGot:\n%s", expectedContent, capturedContent)
	}
}
