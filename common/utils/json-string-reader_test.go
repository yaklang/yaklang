package utils

import (
	"io"
	"strings"
	"testing"
)

func TestJSONStringReader_BasicString(t *testing.T) {
	// 测试基本字符串解码
	input := `"123"`
	expected := "123"

	reader := JSONStringReader(strings.NewReader(input))
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("读取失败: %v", err)
	}

	if string(result) != expected {
		t.Errorf("期望: %q, 实际: %q", expected, string(result))
	}
}

func TestJSONStringReader_StringWithNewline(t *testing.T) {
	// 测试包含换行符的字符串
	input := `"abc\n123"`
	expected := "abc\n123"

	reader := JSONStringReader(strings.NewReader(input))
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("读取失败: %v", err)
	}

	if string(result) != expected {
		t.Errorf("期望: %q, 实际: %q", expected, string(result))
	}
}

func TestJSONStringReader_EscapeSequences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "双引号转义",
			input:    `"He said \"Hello\""`,
			expected: `He said "Hello"`,
		},
		{
			name:     "反斜杠转义",
			input:    `"path\\to\\file"`,
			expected: `path\to\file`,
		},
		{
			name:     "斜杠转义",
			input:    `"url\/path"`,
			expected: `url/path`,
		},
		{
			name:     "退格符",
			input:    `"line\bspace"`,
			expected: "line\bspace",
		},
		{
			name:     "换页符",
			input:    `"page\fbreak"`,
			expected: "page\fbreak",
		},
		{
			name:     "换行符",
			input:    `"line\nbreak"`,
			expected: "line\nbreak",
		},
		{
			name:     "回车符",
			input:    `"carriage\rreturn"`,
			expected: "carriage\rreturn",
		},
		{
			name:     "制表符",
			input:    `"tab\tspace"`,
			expected: "tab\tspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_HexEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "十六进制空格",
			input:    `"hello\x20world"`,
			expected: "hello world",
		},
		{
			name:     "十六进制换行",
			input:    `"line\x0Abreak"`,
			expected: "line\nbreak",
		},
		{
			name:     "十六进制制表符",
			input:    `"tab\x09space"`,
			expected: "tab\tspace",
		},
		{
			name:     "大写十六进制",
			input:    `"hex\x41BC"`,
			expected: "hexABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_UnicodeEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Unicode空格",
			input:    `"hello\u0020world"`,
			expected: "hello world",
		},
		{
			name:     "Unicode中文",
			input:    `"你好\u4e16\u754c"`,
			expected: "你好世界",
		},
		{
			name:     "Unicode表情",
			input:    `"smile\ud83d\ude0a"`,
			expected: "smile😊",
		},
		{
			name:     "Unicode拉丁字符",
			input:    `"caf\u00e9"`,
			expected: "café",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_ComplexEscapes(t *testing.T) {
	// 测试复杂的转义组合
	input := `"Line 1\nLine 2\tTab\rCarriage\x20Space\u0020Unicode\\Backslash\"Quote"`
	expected := "Line 1\nLine 2\tTab\rCarriage Space Unicode\\Backslash\"Quote"

	reader := JSONStringReader(strings.NewReader(input))
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("读取失败: %v", err)
	}

	if string(result) != expected {
		t.Errorf("期望: %q, 实际: %q", expected, string(result))
	}
}

func TestJSONStringReader_Fallback(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "无引号数字",
			input:    `123`,
			expected: `123`,
		},
		{
			name:     "无引号文本",
			input:    `hello world`,
			expected: `hello world`,
		},
		{
			name:     "JSON对象",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON数组",
			input:    `[1, 2, 3]`,
			expected: `[1, 2, 3]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_WithWhitespace(t *testing.T) {
	// 测试前置空白字符
	input := `   "hello world"   `
	expected := "hello world"

	reader := JSONStringReader(strings.NewReader(input))
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("读取失败: %v", err)
	}

	if string(result) != expected {
		t.Errorf("期望: %q, 实际: %q", expected, string(result))
	}
}

func TestJSONStringReader_StreamingRead(t *testing.T) {
	// 测试流式读取
	input := `"This is a very long string that should be read in chunks to test the streaming functionality of the JSON string reader"`
	expected := "This is a very long string that should be read in chunks to test the streaming functionality of the JSON string reader"

	reader := JSONStringReader(strings.NewReader(input))

	// 用小缓冲区分块读取
	var result []byte
	buf := make([]byte, 10) // 小缓冲区
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Errorf("读取失败: %v", err)
			break
		}
	}

	if string(result) != expected {
		t.Errorf("期望: %q, 实际: %q", expected, string(result))
	}
}

func TestJSONStringReader_InvalidEscapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "无效Unicode转义",
			input:    `"invalid\uXXXX"`,
			expected: "invaliduXXXX",
		},
		{
			name:     "不完整Unicode转义",
			input:    `"incomplete\u123"`,
			expected: "incompleteu123",
		},
		{
			name:     "无效十六进制转义",
			input:    `"invalid\xZZ"`,
			expected: "invalidxZZ",
		},
		{
			name:     "不完整十六进制转义",
			input:    `"incomplete\x1"`,
			expected: "incompletex1",
		},
		{
			name:     "无效转义字符",
			input:    `"invalid\z"`,
			expected: "invalidz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_EmptyString(t *testing.T) {
	// 测试空字符串
	input := `""`
	expected := ""

	reader := JSONStringReader(strings.NewReader(input))
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("读取失败: %v", err)
	}

	if string(result) != expected {
		t.Errorf("期望: %q, 实际: %q", expected, string(result))
	}
}

func TestJSONStringReader_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "只有引号",
			input:    `""`,
			expected: "",
		},
		{
			name:     "单个字符",
			input:    `"a"`,
			expected: "a",
		},
		{
			name:     "只有转义字符",
			input:    `"\n"`,
			expected: "\n",
		},
		{
			name:     "连续转义",
			input:    `"\n\r\t"`,
			expected: "\n\r\t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_MalformedInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "多个双引号",
			input:    `"asdfasdfasdf"asdfasdfasdfasdf"""""`,
			expected: `"asdfasdfasdf"asdfasdfasdfasdf"""""`,
		},
		{
			name:     "双引号后有非空白内容",
			input:    `"hello"world`,
			expected: `"hello"world`,
		},
		{
			name:     "双引号后有数字",
			input:    `"test"123`,
			expected: `"test"123`,
		},
		{
			name:     "字符串中有未转义的控制字符",
			input:    "\"hello\x01world\"",
			expected: "\"hello\x01world\"",
		},
		{
			name:     "转义后跟不可打印字符",
			input:    "\"hello\\\x01world\"",
			expected: "\"hello\\\x01world\"",
		},
		{
			name:     "混合正常和异常内容",
			input:    `"normal"then"abnormal"more"text`,
			expected: `"normal"then"abnormal"more"text`,
		},
		{
			name:     "字符串中间有双引号无转义",
			input:    `"hello"world"test"`,
			expected: `"hello"world"test"`,
		},
		{
			name:     "嵌套引号结构",
			input:    `"outer"inner"middle"end`,
			expected: `"outer"inner"middle"end`,
		},
		{
			name:     "JSON对象中的引号错误",
			input:    `{"key": "value"extra"}`,
			expected: `{"key": "value"extra"}`,
		},
		{
			name:     "转义错误后接普通内容",
			input:    `"test\q normal"`,
			expected: "testq normal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_MalformedStreaming(t *testing.T) {
	// 测试流式读取畸形数据
	input := `"valid start"then invalid content with "multiple" quotes "everywhere"`
	expected := input

	reader := JSONStringReader(strings.NewReader(input))

	// 用小缓冲区分块读取
	var result []byte
	buf := make([]byte, 5) // 很小的缓冲区
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Errorf("读取失败: %v", err)
			break
		}
	}

	if string(result) != expected {
		t.Errorf("期望: %q, 实际: %q", expected, string(result))
	}
}

func TestJSONStringReader_PartialMalformed(t *testing.T) {
	// 测试部分解析后发现畸形的情况
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "解析过程中发现多余引号",
			input:    `"hello world" extra content`,
			expected: `"hello world" extra content`,
		},
		{
			name:     "转义处理后发现异常",
			input:    `"escaped\n content" and more`,
			expected: `"escaped\n content" and more`,
		},
		{
			name:     "Unicode处理后发现异常",
			input:    `"unicode\u0020test" additional`,
			expected: `"unicode\u0020test" additional`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_UserSpecificCases(t *testing.T) {
	// 测试用户提到的具体畸形情况
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "用户示例：多个双引号",
			input:    `"asdfasdfasdf"asdfasdfasdfasdf"""""`,
			expected: `"asdfasdfasdf"asdfasdfasdfasdf"""""`,
		},
		{
			name:     "复杂畸形：混合引号",
			input:    `"start"middle"more"end"final"`,
			expected: `"start"middle"more"end"final"`,
		},
		{
			name:     "转义错误后的畸形",
			input:    `"hello\invalid"world"`,
			expected: `"hello\invalid"world"`,
		},
		{
			name:     "JSON对象畸形",
			input:    `{"key": "value"malformed"}: more`,
			expected: `{"key": "value"malformed"}: more`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_WhitespaceHandling(t *testing.T) {
	// 测试各种空白字符的处理
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "前置空格",
			input:    `   "hello world"`,
			expected: "hello world",
		},
		{
			name:     "前置制表符",
			input:    "\t\t\"test content\"",
			expected: "test content",
		},
		{
			name:     "前置换行符",
			input:    "\n\n\"line content\"",
			expected: "line content",
		},
		{
			name:     "前置回车符",
			input:    "\r\r\"carriage return\"",
			expected: "carriage return",
		},
		{
			name:     "前置垂直制表符",
			input:    "\v\"vertical tab\"",
			expected: "vertical tab",
		},
		{
			name:     "前置换页符",
			input:    "\f\"form feed\"",
			expected: "form feed",
		},
		{
			name:     "混合空白字符",
			input:    " \t\r\n\v\f \"mixed whitespace\"",
			expected: "mixed whitespace",
		},
		{
			name:     "大量空白字符",
			input:    strings.Repeat(" ", 100) + `"lots of spaces"`,
			expected: "lots of spaces",
		},
		{
			name:     "空白后非JSON",
			input:    "   hello world",
			expected: "   hello world",
		},
		{
			name:     "制表符后非JSON",
			input:    "\t\tplain text",
			expected: "\t\tplain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_BoundaryConditions(t *testing.T) {
	// 测试边界条件和防panic - 主要目标是确保不会panic
	tests := []struct {
		name     string
		input    string
		testFunc func(t *testing.T, input string, result []byte, err error)
	}{
		{
			name:  "空输入",
			input: "",
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				if err != nil {
					t.Errorf("不应该出错: %v", err)
				}
			},
		},
		{
			name:  "只有一个字符",
			input: "a",
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				if err != nil {
					t.Errorf("不应该出错: %v", err)
				}
				if string(result) != "a" {
					t.Errorf("期望: %q, 实际: %q", "a", string(result))
				}
			},
		},
		{
			name:  "只有一个引号",
			input: `"`,
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				// 主要确保不panic，结果可能为空或者是原始输入
				if err != nil && err != io.EOF {
					t.Errorf("不应该出现非EOF错误: %v", err)
				}
			},
		},
		{
			name:  "不完整Unicode转义",
			input: `"test\u"`,
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				// 主要确保不panic
				if err != nil && err != io.EOF {
					t.Errorf("不应该出现非EOF错误: %v", err)
				}
			},
		},
		{
			name:  "不完整十六进制转义",
			input: `"test\x"`,
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				// 主要确保不panic
				if err != nil && err != io.EOF {
					t.Errorf("不应该出现非EOF错误: %v", err)
				}
			},
		},
		{
			name:  "截断的Unicode",
			input: `"test\u12"`,
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				// 主要确保不panic
				if err != nil && err != io.EOF {
					t.Errorf("不应该出现非EOF错误: %v", err)
				}
			},
		},
		{
			name:  "截断的十六进制",
			input: `"test\x1"`,
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				if err != nil {
					t.Errorf("不应该出错: %v", err)
				}
				if string(result) != "testx1" {
					t.Errorf("期望: %q, 实际: %q", "testx1", string(result))
				}
			},
		},
		{
			name:  "转义在末尾",
			input: `"test\`,
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				// 主要确保不panic
				if err != nil && err != io.EOF {
					t.Errorf("不应该出现非EOF错误: %v", err)
				}
			},
		},
		{
			name:  "只有空白字符",
			input: "   \t\r\n  ",
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				if err != nil && err != io.EOF {
					t.Errorf("不应该出现非EOF错误: %v", err)
				}
				// 只有空白字符的情况下，reader会等待更多数据，最终返回空或EOF
				// 主要确保不panic即可
			},
		},
		{
			name:  "非常长的字符串",
			input: `"` + strings.Repeat("a", 10000) + `"`,
			testFunc: func(t *testing.T, input string, result []byte, err error) {
				if err != nil {
					t.Errorf("不应该出错: %v", err)
				}
				if string(result) != strings.Repeat("a", 10000) {
					t.Errorf("长字符串处理失败")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := JSONStringReader(strings.NewReader(tt.input))
			result, err := io.ReadAll(reader)
			tt.testFunc(t, tt.input, result, err)
		})
	}
}

func TestJSONStringReader_StressTest(t *testing.T) {
	// 压力测试，防止panic
	t.Run("随机数据压力测试", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			// 生成随机长度的随机数据
			length := i*10 + 1
			data := make([]byte, length)
			for j := range data {
				data[j] = byte(j % 256) // 包含所有可能的字节值
			}

			reader := JSONStringReader(strings.NewReader(string(data)))
			// 确保不会panic
			_, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("压力测试失败 %d: %v", i, err)
			}
		}
	})

	t.Run("极小缓冲区读取", func(t *testing.T) {
		input := `"hello\nworld\u0020test\x41"and more"malformed"`
		reader := JSONStringReader(strings.NewReader(input))

		// 使用1字节的缓冲区逐字节读取
		var result []byte
		buf := make([]byte, 1)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				result = append(result, buf[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("极小缓冲区测试失败: %v", err)
				break
			}
		}

		if len(result) == 0 {
			t.Error("极小缓冲区测试未读取任何数据")
		}
	})

	t.Run("零长度读取", func(t *testing.T) {
		input := `"test"`
		reader := JSONStringReader(strings.NewReader(input))

		// 测试零长度读取
		buf := make([]byte, 0)
		n, err := reader.Read(buf)
		if n != 0 || err != nil {
			t.Errorf("零长度读取失败: n=%d, err=%v", n, err)
		}
	})

	t.Run("并发读取测试", func(t *testing.T) {
		input := `"concurrent test data"`

		// 并发创建多个reader
		for i := 0; i < 10; i++ {
			go func(index int) {
				reader := JSONStringReader(strings.NewReader(input))
				result, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("并发测试 %d 失败: %v", index, err)
				}
				if string(result) != "concurrent test data" {
					t.Errorf("并发测试 %d 结果错误: %s", index, string(result))
				}
			}(i)
		}
	})
}

// chunkReader 按预设分片返回数据，模拟网络流/io.Pipe 那样的逐块到达，
// 用于复现转义序列跨 Read 边界的场景。
type chunkReader struct {
	chunks [][]byte
	idx    int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.idx >= len(c.chunks) {
		return 0, io.EOF
	}
	chunk := c.chunks[c.idx]
	c.idx++
	n := copy(p, chunk)
	// 若 p 装不下整块，剩余部分留到下一块前面（极小缓冲场景）
	if n < len(chunk) {
		c.chunks[c.idx-1] = chunk[n:]
		c.idx--
	}
	return n, nil
}

func TestJSONStringReader_ChunkedEscapeAcrossBoundary(t *testing.T) {
	// 回归测试：转义序列跨分片到达时不得重复输出已处理的前缀。
	// 这类场景在 io.Pipe / 网络流（如 LLM 流式响应）中非常常见。
	tests := []struct {
		name     string
		chunks   []string
		expected string
	}{
		{
			name:     "unicode转义跨分片",
			chunks:   []string{`"Hello from Yak AI\u00`, `21"`},
			expected: "Hello from Yak AI!",
		},
		{
			name:     "十六进制转义跨分片",
			chunks:   []string{`"hello\x`, `20world"`},
			expected: "hello world",
		},
		{
			name:     "代理对跨分片",
			chunks:   []string{`"smile\ud8`, `3d\ude0a"`},
			expected: "smile😊",
		},
		{
			name:     "反斜杠转义跨分片",
			chunks:   []string{`"line\`, `nbreak"`},
			expected: "line\nbreak",
		},
		{
			name:     "多个转义逐字符到达",
			chunks:   []string{`"a\u00`, `41b\x`, `42c\u00`, `43"`},
			expected: "aAbBcC",
		},
		{
			name:     "中文unicode跨分片",
			chunks:   []string{`"你好\u4e1`, `6\u754c"`},
			expected: "你好世界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := make([][]byte, 0, len(tt.chunks))
			for _, c := range tt.chunks {
				chunks = append(chunks, []byte(c))
			}
			reader := JSONStringReader(&chunkReader{chunks: chunks})
			result, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("读取失败: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("期望: %q, 实际: %q", tt.expected, string(result))
			}
		})
	}
}

func TestJSONStringReader_ChunkedViaPipe(t *testing.T) {
	// 用真实 io.Pipe 复现：生产者分多片写出，消费者用小缓冲读取，转义恰好跨片
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for _, chunk := range []string{`"Hello`, ` from`, ` Yak`, ` AI\u00`, `21"`} {
			pw.Write([]byte(chunk))
		}
	}()

	reader := JSONStringReader(pr)
	var result []byte
	buf := make([]byte, 4) // 小缓冲，逼出分片读取路径
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Errorf("读取失败: %v", err)
			break
		}
	}
	if string(result) != "Hello from Yak AI!" {
		t.Errorf("期望: %q, 实际: %q", "Hello from Yak AI!", string(result))
	}
}

func TestJSONStringReader_ExtremeCases(t *testing.T) {
	// 极端情况测试
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "全是控制字符",
			input: string([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}),
		},
		{
			name:  "全是高位字节",
			input: string([]byte{128, 129, 130, 255, 254, 253}),
		},
		{
			name:  "混合二进制数据",
			input: "\"test\"\x00\x01\xff\"more\"",
		},
		{
			name:  "超长转义序列",
			input: `"test` + strings.Repeat(`\u`, 100) + `"`,
		},
		{
			name:  "嵌套多层引号",
			input: strings.Repeat(`"`, 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 这些测试主要确保不会panic
			reader := JSONStringReader(strings.NewReader(tt.input))
			_, err := io.ReadAll(reader)
			// 允许任何结果，只要不panic
			_ = err // 忽略错误，我们只关心不要panic
		})
	}
}
