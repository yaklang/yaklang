package aitag

import (
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

func TestParseSimpleTag(t *testing.T) {
	input := `前面一些内容
<|CODE_abc123|>
这是代码内容
func main() {
    fmt.Println("hello world")
}
<|CODE_END_abc123|>
后面一些内容`

	var capturedContent string
	err := Parse(strings.NewReader(input), WithCallback("CODE", "abc123", func(reader io.Reader) {
		content, _ := io.ReadAll(reader)
		capturedContent = string(content)
		log.Infof("captured content: %s", capturedContent)
	}))

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	expectedContent := `
这是代码内容
func main() {
    fmt.Println("hello world")
}
`
	if strings.TrimSpace(capturedContent) != strings.TrimSpace(expectedContent) {
		t.Errorf("Expected content:\n%s\nGot:\n%s", expectedContent, capturedContent)
	}
}

func TestParseMultipleTags(t *testing.T) {
	input := `开始内容
<|CODE_nonce1|>
第一段代码
package main
<|CODE_END_nonce1|>
中间内容
<|DATA_nonce2|>
一些数据内容
key: value
<|DATA_END_nonce2|>
结束内容`

	var codeContent, dataContent string

	err := Parse(strings.NewReader(input),
		WithCallback("CODE", "nonce1", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			codeContent = string(content)
		}),
		WithCallback("DATA", "nonce2", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			dataContent = string(content)
		}),
	)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !strings.Contains(codeContent, "第一段代码") || !strings.Contains(codeContent, "package main") {
		t.Errorf("Code content not captured correctly: %s", codeContent)
	}

	if !strings.Contains(dataContent, "一些数据内容") || !strings.Contains(dataContent, "key: value") {
		t.Errorf("Data content not captured correctly: %s", dataContent)
	}
}

func TestParseNestedTags(t *testing.T) {
	// Note: This parser does NOT support nested tags
	// The inner tag should be treated as content of the outer tag
	input := `外层内容
<|OUTER_test1|>
外层标签内容
<|INNER_test2|>
内层标签内容
<|INNER_END_test2|>
更多外层内容
<|OUTER_END_test1|>
结束内容`

	var outerContent string
	var innerCallbackTriggered bool

	err := Parse(strings.NewReader(input),
		WithCallback("OUTER", "test1", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			outerContent = string(content)
		}),
		WithCallback("INNER", "test2", func(reader io.Reader) {
			// This should NOT be triggered because nesting is not supported
			innerCallbackTriggered = true
		}),
	)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Inner callback should NOT be triggered
	if innerCallbackTriggered {
		t.Error("Inner callback should not be triggered as nesting is not supported")
	}

	// Outer content should contain the inner tags as plain text
	if !strings.Contains(outerContent, "外层标签内容") || !strings.Contains(outerContent, "更多外层内容") {
		t.Errorf("Outer content not captured correctly: %s", outerContent)
	}

	// The inner tags should be included as text content
	if !strings.Contains(outerContent, "<|INNER_test2|>") || !strings.Contains(outerContent, "<|INNER_END_test2|>") {
		t.Error("Inner tags should be included as plain text in outer content")
	}
}

func TestParseTagOnSameLine(t *testing.T) {
	input := `<|INLINE_xyz|>内容在同一行<|INLINE_END_xyz|>`

	var capturedContent string
	err := Parse(strings.NewReader(input), WithCallback("INLINE", "xyz", func(reader io.Reader) {
		content, _ := io.ReadAll(reader)
		capturedContent = string(content)
	}))

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !strings.Contains(capturedContent, "内容在同一行") {
		t.Errorf("Inline content not captured correctly: %s", capturedContent)
	}
}

func TestParseWithoutMatchingCallback(t *testing.T) {
	input := `<|UNKNOWN_tag|>
这个标签没有注册回调
<|UNKNOWN_END_tag|>`

	// Should not panic or error, just ignore the unregistered tag
	err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse should not fail for unregistered tags: %v", err)
	}
}

func TestParseStreamingContent(t *testing.T) {
	// Test with a reader that simulates streaming content
	content := `<|STREAM_test123|>
第一行
第二行
第三行
<|STREAM_END_test123|>`

	var results []string
	err := Parse(strings.NewReader(content), WithCallback("STREAM", "test123", func(reader io.Reader) {
		buf := make([]byte, 1024)
		n, _ := reader.Read(buf)
		results = append(results, string(buf[:n]))
	}))

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 callback execution, got %d", len(results))
	}

	content_str := results[0]
	if !strings.Contains(content_str, "第一行") || !strings.Contains(content_str, "第二行") || !strings.Contains(content_str, "第三行") {
		t.Errorf("Streaming content not captured correctly: %s", content_str)
	}
}

func TestParseWithCallbacks(t *testing.T) {
	input := `<|TYPE1_nonce1|>
内容1
<|TYPE1_END_nonce1|>
<|TYPE2_nonce2|>
内容2
<|TYPE2_END_nonce2|>`

	callbacks := map[string]map[string]CallbackFunc{
		"TYPE1": {
			"nonce1": func(reader io.Reader) {
				content, _ := io.ReadAll(reader)
				if !strings.Contains(string(content), "内容1") {
					t.Error("TYPE1 content not found")
				}
			},
		},
		"TYPE2": {
			"nonce2": func(reader io.Reader) {
				content, _ := io.ReadAll(reader)
				if !strings.Contains(string(content), "内容2") {
					t.Error("TYPE2 content not found")
				}
			},
		},
	}

	err := ParseWithCallbacks(strings.NewReader(input), callbacks)
	if err != nil {
		t.Fatalf("ParseWithCallbacks failed: %v", err)
	}
}
