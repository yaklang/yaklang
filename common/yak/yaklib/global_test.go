package yaklib

import "testing"

func TestOrd(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{
			name:     "rune A",
			input:    'A',
			expected: 65,
		},
		{
			name:     "string A",
			input:    "A",
			expected: 65,
		},
		{
			name:     "rune a",
			input:    'a',
			expected: 97,
		},
		{
			name:     "string a",
			input:    "a",
			expected: 97,
		},
		{
			name:     "rune 0",
			input:    '0',
			expected: 48,
		},
		{
			name:     "string 0",
			input:    "0",
			expected: 48,
		},
		{
			name:     "rune space",
			input:    ' ',
			expected: 32,
		},
		{
			name:     "string space",
			input:    " ",
			expected: 32,
		},
		// 特殊字符测试
		{
			name:     "rune newline",
			input:    '\n',
			expected: 10,
		},
		{
			name:     "string newline",
			input:    "\n",
			expected: 10,
		},
		{
			name:     "rune tab",
			input:    '\t',
			expected: 9,
		},
		{
			name:     "string tab",
			input:    "\t",
			expected: 9,
		},
		// byte 类型测试
		{
			name:     "byte 65",
			input:    byte(65),
			expected: 65,
		},
		{
			name:     "byte 97",
			input:    byte(97),
			expected: 97,
		},
		// 多字符字符串测试（只取第一个字符）
		{
			name:     "string Hello",
			input:    "Hello",
			expected: 72, // 'H' 的 ASCII 码
		},
		{
			name:     "string abc",
			input:    "abc",
			expected: 97, // 'a' 的 ASCII 码
		},
		// Unicode 字符测试
		{
			name:     "rune unicode ©",
			input:    '©',
			expected: 169,
		},
		{
			name:     "string unicode ©",
			input:    "©",
			expected: 169,
		},
		{
			name:     "rune unicode 中",
			input:    '中',
			expected: 20013,
		},
		{
			name:     "string unicode 中",
			input:    "中",
			expected: 20013,
		},
		// 边界测试
		{
			name:     "rune null",
			input:    '\x00',
			expected: 0,
		},
		{
			name:     "string null",
			input:    "\x00",
			expected: 0,
		},
		{
			name:     "rune DEL",
			input:    '\x7F',
			expected: 127,
		},
		// 空字符串测试
		{
			name:     "empty string",
			input:    "",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ord(tt.input)
			if result != tt.expected {
				t.Errorf("ord(%v) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}
