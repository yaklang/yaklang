package aitag

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

// TestNoNestingSupport 测试不支持嵌套标签
func TestNoNestingSupport(t *testing.T) {
	input := `开始测试
<|OUTER_test1|>
外层内容开始
<|INNER_test2|>
这应该被当作外层的内容，而不是内层标签
<|INNER_END_test2|>
外层内容结束
<|OUTER_END_test1|>
测试结束`

	var outerContent string
	var innerCallbackTriggered bool

	err := Parse(strings.NewReader(input),
		WithCallback("OUTER", "test1", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			outerContent = string(content)
			log.Infof("外层标签内容: %s", outerContent)
		}),
		WithCallback("INNER", "test2", func(reader io.Reader) {
			innerCallbackTriggered = true
			content, _ := io.ReadAll(reader)
			log.Infof("内层标签应该不会被触发，但收到了: %s", string(content))
		}),
	)

	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 内层回调不应该被触发
	if innerCallbackTriggered {
		t.Error("内层标签回调不应该被触发，因为不支持嵌套")
	}

	// 外层内容应该包含所有文本，包括内层标签
	if !strings.Contains(outerContent, "<|INNER_test2|>") {
		t.Error("外层内容应该包含内层标签作为普通文本")
	}
	if !strings.Contains(outerContent, "<|INNER_END_test2|>") {
		t.Error("外层内容应该包含内层结束标签作为普通文本")
	}
	if !strings.Contains(outerContent, "这应该被当作外层的内容") {
		t.Error("外层内容应该包含所有中间文本")
	}

	log.Infof("嵌套测试完成，外层内容长度: %d", len(outerContent))
}

// TestMultipleSequentialTags 测试顺序的多个标签
func TestMultipleSequentialTags(t *testing.T) {
	input := `开始
<|TAG1_nonce1|>
第一个标签的内容
包含多行
<|TAG1_END_nonce1|>
中间文本
<|TAG2_nonce2|>
第二个标签的内容
也包含多行
<|TAG2_END_nonce2|>
<|TAG3_nonce3|>
第三个标签
<|TAG3_END_nonce3|>
结束`

	var results = make(map[string]string)
	var mu sync.Mutex

	err := Parse(strings.NewReader(input),
		WithCallback("TAG1", "nonce1", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			mu.Lock()
			results["tag1"] = string(content)
			mu.Unlock()
			log.Infof("TAG1 处理完成")
		}),
		WithCallback("TAG2", "nonce2", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			mu.Lock()
			results["tag2"] = string(content)
			mu.Unlock()
			log.Infof("TAG2 处理完成")
		}),
		WithCallback("TAG3", "nonce3", func(reader io.Reader) {
			content, _ := io.ReadAll(reader)
			mu.Lock()
			results["tag3"] = string(content)
			mu.Unlock()
			log.Infof("TAG3 处理完成")
		}),
	)

	if err != nil {
		t.Fatalf("顺序标签解析失败: %v", err)
	}

	// 所有三个标签都应该被处理
	if len(results) != 3 {
		t.Errorf("期望处理3个标签，实际处理了 %d 个", len(results))
	}

	// 验证内容
	if !strings.Contains(results["tag1"], "第一个标签的内容") {
		t.Error("TAG1 内容不正确")
	}
	if !strings.Contains(results["tag2"], "第二个标签的内容") {
		t.Error("TAG2 内容不正确")
	}
	if !strings.Contains(results["tag3"], "第三个标签") {
		t.Error("TAG3 内容不正确")
	}

	log.Infof("顺序多标签测试完成")
}

// TestInvalidNestedTags 测试各种无效的嵌套情况
func TestInvalidNestedTags(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectOuter bool
		expectInner bool
		description string
	}{
		{
			name: "深度嵌套",
			input: `<|OUTER_test|>
开始
<|MIDDLE_test|>
中间
<|INNER_test|>
最内层
<|INNER_END_test|>
<|MIDDLE_END_test|>
<|OUTER_END_test|>`,
			expectOuter: true,
			expectInner: false,
			description: "三层嵌套，只有最外层应该工作",
		},
		{
			name: "相同标签嵌套",
			input: `<|SAME_test1|>
外层内容
<|SAME_test2|>
内层内容
<|SAME_END_test2|>
<|SAME_END_test1|>`,
			expectOuter: true,
			expectInner: false,
			description: "相同标签名但不同nonce的嵌套",
		},
		{
			name: "不完整嵌套",
			input: `<|OUTER_test|>
外层开始
<|INNER_test|>
内层但没有结束
<|OUTER_END_test|>`,
			expectOuter: true,
			expectInner: false,
			description: "外层正常结束，内层未结束",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var outerTriggered, innerTriggered bool
			var outerContent, innerContent string

			err := Parse(strings.NewReader(tc.input),
				WithCallback("OUTER", "test", func(reader io.Reader) {
					outerTriggered = true
					content, _ := io.ReadAll(reader)
					outerContent = string(content)
				}),
				WithCallback("SAME", "test1", func(reader io.Reader) {
					outerTriggered = true
					content, _ := io.ReadAll(reader)
					outerContent = string(content)
				}),
				WithCallback("MIDDLE", "test", func(reader io.Reader) {
					innerTriggered = true
					content, _ := io.ReadAll(reader)
					innerContent = string(content)
				}),
				WithCallback("INNER", "test", func(reader io.Reader) {
					innerTriggered = true
					content, _ := io.ReadAll(reader)
					innerContent = string(content)
				}),
				WithCallback("SAME", "test2", func(reader io.Reader) {
					innerTriggered = true
					content, _ := io.ReadAll(reader)
					innerContent = string(content)
				}),
			)

			if err != nil {
				t.Errorf("解析失败 [%s]: %v", tc.name, err)
			}

			if outerTriggered != tc.expectOuter {
				t.Errorf("[%s] 外层标签触发状态不符合预期: 期望 %v, 实际 %v", tc.name, tc.expectOuter, outerTriggered)
			}

			if innerTriggered != tc.expectInner {
				t.Errorf("[%s] 内层标签触发状态不符合预期: 期望 %v, 实际 %v", tc.name, tc.expectInner, innerTriggered)
			}

			log.Infof("无效嵌套测试 [%s] 完成: %s", tc.name, tc.description)
			if outerTriggered {
				log.Infof("外层内容长度: %d", len(outerContent))
			}
			if innerTriggered {
				log.Infof("内层内容长度: %d", len(innerContent))
			}
		})
	}
}

// TestMalformedTags 测试格式错误的标签
func TestMalformedTags(t *testing.T) {
	input := `正常内容
<|GOOD_test|>
正常标签内容
<|GOOD_END_test|>
更多内容
<|BAD_test
这是一个没有正确结束的标签
<|ANOTHER_GOOD_test|>
另一个正常标签
<|ANOTHER_GOOD_END_test|>
<NOTAG>这不是标签</NOTAG>
结束`

	var goodCount int
	var results = make(map[string]string)

	err := Parse(strings.NewReader(input),
		WithCallback("GOOD", "test", func(reader io.Reader) {
			goodCount++
			content, _ := io.ReadAll(reader)
			results["good"] = string(content)
			log.Infof("GOOD 标签处理完成")
		}),
		WithCallback("ANOTHER_GOOD", "test", func(reader io.Reader) {
			goodCount++
			content, _ := io.ReadAll(reader)
			results["another_good"] = string(content)
			log.Infof("ANOTHER_GOOD 标签处理完成")
		}),
		WithCallback("BAD", "test", func(reader io.Reader) {
			t.Error("BAD 标签不应该被触发，因为格式错误")
		}),
	)

	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 应该只有两个正常标签被处理
	if goodCount != 2 {
		t.Errorf("期望处理2个正常标签，实际处理了 %d 个", goodCount)
	}

	// 验证正常标签内容
	if !strings.Contains(results["good"], "正常标签内容") {
		t.Error("GOOD 标签内容不正确")
	}
	if !strings.Contains(results["another_good"], "另一个正常标签") {
		t.Error("ANOTHER_GOOD 标签内容不正确")
	}

	log.Infof("格式错误标签测试完成")
}

// TestLargeContentWithoutNesting 测试大内容且无嵌套
func TestLargeContentWithoutNesting(t *testing.T) {
	// 生成大量内容
	var builder strings.Builder
	builder.WriteString("<|LARGE_test|>\n")

	// 添加大量行，包含一些看起来像标签但实际不是的内容
	for i := 0; i < 1000; i++ {
		builder.WriteString(fmt.Sprintf("行 %d: 这里有一些内容 <not_a_tag> 和其他数据\n", i))
		if i%100 == 0 {
			builder.WriteString("这里有一些看起来像标签的内容: <|FAKE_fake|> 但实际不是\n")
		}
	}

	builder.WriteString("<|LARGE_END_test|>\n")

	var contentReceived bool
	var contentSize int

	err := Parse(strings.NewReader(builder.String()), WithCallback("LARGE", "test", func(reader io.Reader) {
		contentReceived = true
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("读取大内容失败: %v", err)
		}
		contentSize = len(content)

		// 验证内容包含预期的行
		contentStr := string(content)
		if !strings.Contains(contentStr, "行 0:") {
			t.Error("大内容应该包含第一行")
		}
		if !strings.Contains(contentStr, "行 999:") {
			t.Error("大内容应该包含最后一行")
		}
		if !strings.Contains(contentStr, "<|FAKE_fake|>") {
			t.Error("大内容应该包含假标签作为普通文本")
		}

		log.Infof("大内容处理完成，大小: %d 字节", contentSize)
	}))

	if err != nil {
		t.Fatalf("大内容解析失败: %v", err)
	}

	if !contentReceived {
		t.Error("大内容回调未被触发")
	}

	if contentSize == 0 {
		t.Error("接收到的大内容为空")
	}

	log.Infof("大内容测试完成，处理了 %d 字节", contentSize)
}
