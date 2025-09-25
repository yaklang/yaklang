package aitag

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

// ExampleParse demonstrates how to use the aitag parser
func ExampleParse() {
	input := `这是一些普通文本
<|CODE_abc123|>
package main

import "fmt"

func main() {
    fmt.Println("Hello World")
}
<|CODE_END_abc123|>
还有更多文本

<|DATA_xyz789|>
{
    "name": "example",
    "version": "1.0"
}
<|DATA_END_xyz789|>

结束文本`

	err := Parse(strings.NewReader(input),
		WithCallback("CODE", "abc123", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			fmt.Printf("代码内容:\n%s\n", string(content))
		}),
		WithCallback("DATA", "xyz789", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			fmt.Printf("数据内容:\n%s\n", string(content))
		}),
	)

	if err != nil {
		log.Errorf("解析失败: %v", err)
	}
}

// TestExampleUsage demonstrates real-world usage scenarios without nesting
func TestExampleUsage(t *testing.T) {
	// 测试流式解析多个顺序标签 (不支持嵌套)
	complexInput := `开始处理请求
<|REQUEST_req001|>
{
    "method": "POST",
    "url": "/api/data", 
    "timestamp": "2023-01-01T00:00:00Z"
}
<|REQUEST_END_req001|>

<|HEADERS_req001|>
Content-Type: application/json
Authorization: Bearer token123
User-Agent: aitag-parser/1.0
<|HEADERS_END_req001|>

<|BODY_req001|>
{
    "user": "alice",
    "action": "create",
    "data": {
        "name": "测试项目",
        "description": "这是一个测试项目",
        "tags": ["test", "demo"]
    }
}
<|BODY_END_req001|>

<|METADATA_req001|>
{
    "version": "1.0",
    "encoding": "utf-8",
    "processed_by": "aitag-parser"
}
<|METADATA_END_req001|>
处理完成`

	var results = make(map[string]string)

	err := Parse(strings.NewReader(complexInput),
		WithCallback("REQUEST", "req001", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			results["request"] = string(content)
			log.Infof("收到完整请求数据")
		}),
		WithCallback("HEADERS", "req001", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			results["headers"] = string(content)
			log.Infof("收到请求头数据")
		}),
		WithCallback("BODY", "req001", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			results["body"] = string(content)
			log.Infof("收到请求体数据")
		}),
		WithCallback("METADATA", "req001", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			results["metadata"] = string(content)
			log.Infof("收到元数据")
		}),
	)

	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 验证所有预期的内容都被捕获
	expectedKeys := []string{"request", "headers", "body", "metadata"}
	for _, key := range expectedKeys {
		if content, exists := results[key]; !exists {
			t.Errorf("缺少预期的内容: %s", key)
		} else {
			t.Logf("%s 内容长度: %d", key, len(content))
		}
	}

	// 验证内容
	if !strings.Contains(results["body"], "测试项目") {
		t.Error("请求体数据内容不正确")
	}
	if !strings.Contains(results["headers"], "Content-Type") {
		t.Error("请求头内容不正确")
	}
	if !strings.Contains(results["metadata"], "aitag-parser") {
		t.Error("元数据内容不正确")
	}
}

// TestStreamingUsage demonstrates usage with streaming data
func TestStreamingUsage(t *testing.T) {
	// 模拟流式数据处理
	streamInput := `开始流式处理
<|STREAM_stream001|>
第一块数据
第二块数据
第三块数据
<|STREAM_END_stream001|>

<|ANOTHER_STREAM_stream002|>
另一个流的数据
包含多行内容
和各种字符: !@#$%^&*()
<|ANOTHER_STREAM_END_stream002|>
流式处理结束`

	var streamResults []string
	var anotherStreamResults []string

	err := Parse(strings.NewReader(streamInput),
		WithCallback("STREAM", "stream001", func(reader io.Reader) {
			// 模拟按行处理流数据
			content, _ := io.ReadAll(reader)
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					streamResults = append(streamResults, strings.TrimSpace(line))
				}
			}
			log.Infof("处理了 %d 行流数据", len(streamResults))
		}),
		WithCallback("ANOTHER_STREAM", "stream002", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					anotherStreamResults = append(anotherStreamResults, strings.TrimSpace(line))
				}
			}
			log.Infof("处理了另一个流的 %d 行数据", len(anotherStreamResults))
		}),
	)

	if err != nil {
		t.Fatalf("流式解析失败: %v", err)
	}

	// 验证流数据
	if len(streamResults) < 3 {
		t.Errorf("期望至少3行流数据，实际得到 %d 行", len(streamResults))
	}
	if len(anotherStreamResults) < 3 {
		t.Errorf("期望另一个流至少3行数据，实际得到 %d 行", len(anotherStreamResults))
	}

	// 验证内容
	found := false
	for _, line := range anotherStreamResults {
		if strings.Contains(line, "!@#$%^&*()") {
			found = true
			break
		}
	}
	if !found {
		t.Error("特殊字符内容未找到")
	}
}
