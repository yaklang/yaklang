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
