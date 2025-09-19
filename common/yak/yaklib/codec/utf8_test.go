package codec

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/memfile"
)

func TestIsUTF8_MemFile(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "valid utf8 english",
			input:    []byte("hello world"),
			expected: true,
		},
		{
			name:     "valid utf8 chinese",
			input:    []byte("你好世界"),
			expected: true,
		},
		{
			name:     "valid utf8 mixed",
			input:    []byte("hello 世界 ♥"),
			expected: true,
		},
		{
			name:     "empty",
			input:    []byte(""),
			expected: true,
		},
		{
			name:     "invalid utf8",
			input:    []byte{0xff, 0xfe, 0xfd},
			expected: false,
		},
		{
			name:     "partial utf8",
			input:    []byte{0xe4, 0xb8}, // 不完整的中文字符
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf := memfile.New(tt.input)
			result, err := IsUTF8(mf)
			if err != nil {
				t.Fatalf("IsUTF8() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("IsUTF8() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsUTF8_Bytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "valid utf8 bytes",
			input:    []byte("hello 世界"),
			expected: true,
		},
		{
			name:     "invalid utf8 bytes",
			input:    []byte{0xff, 0xfe, 0xfd},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := IsUTF8(tt.input)
			if err != nil {
				t.Fatalf("IsUTF8() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("IsUTF8() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsUTF8_Reader(t *testing.T) {
	validText := "hello 世界 ♥"
	reader := strings.NewReader(validText)

	result, err := IsUTF8(reader)
	if err != nil {
		t.Fatalf("IsUTF8() error = %v", err)
	}
	if !result {
		t.Errorf("IsUTF8() = %v, want %v", result, true)
	}
}

func TestIsUTF8File_SmallFile(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "utf8_test_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// 写入UTF-8内容 (小文件 < 512字节)
	content := "hello 世界 ♥"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	result, err := IsUTF8File(tmpFile.Name())
	if err != nil {
		t.Fatalf("IsUTF8File() error = %v", err)
	}
	if !result {
		t.Errorf("IsUTF8File() = %v, want %v", result, true)
	}
}

func TestIsUTF8File_MediumFile(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "utf8_test_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// 写入UTF-8内容 (中等文件 512-1024字节)
	content := strings.Repeat("hello 世界 ♥ ", 50) // 约800字节
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	result, err := IsUTF8File(tmpFile.Name())
	if err != nil {
		t.Fatalf("IsUTF8File() error = %v", err)
	}
	if !result {
		t.Errorf("IsUTF8File() = %v, want %v", result, true)
	}
}

func TestIsUTF8File_LargeFile(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "utf8_test_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// 写入UTF-8内容 (大文件 > 1024字节)
	content := strings.Repeat("hello 世界 ♥ 这是一个测试文件，包含各种UTF-8字符。", 100) // 约5KB
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	result, err := IsUTF8File(tmpFile.Name())
	if err != nil {
		t.Fatalf("IsUTF8File() error = %v", err)
	}
	if !result {
		t.Errorf("IsUTF8File() = %v, want %v", result, true)
	}
}

func TestFixUTF8Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "complete utf8",
			input:    []byte("hello 世界"),
			expected: "hello 世界",
		},
		{
			name:     "empty input",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "incomplete at start",
			input:    []byte{0xb8, 0xad, 0xe4, 0xb8, 0x96, 0xe7, 0x95, 0x8c}, // 缺少开头字节的"世界"
			expected: "世界",
		},
		{
			name:     "incomplete at end",
			input:    []byte{0xe4, 0xb8, 0x96, 0xe7, 0x95, 0x8c, 0xe4, 0xb8}, // "世界" + 不完整的字符
			expected: "世界",
		},
		{
			name:     "very small valid",
			input:    []byte("a"),
			expected: "a",
		},
		{
			name:     "very small invalid",
			input:    []byte{0xff},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixUTF8Boundaries(tt.input)
			if string(result) != tt.expected {
				t.Errorf("fixUTF8Boundaries() = %q, want %q", string(result), tt.expected)
			}
		})
	}
}

func TestFindSafeStartPosition(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected int
	}{
		{
			name:     "normal start",
			input:    []byte("hello"),
			expected: 0,
		},
		{
			name:     "utf8 start",
			input:    []byte("世界"),
			expected: 0,
		},
		{
			name:     "broken start",
			input:    []byte{0xb8, 0xad, 0xe4, 0xb8, 0x96}, // 缺少开头的字节
			expected: 2,                                    // 应该跳到 0xe4 的位置
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSafeStartPosition(tt.input)
			if result != tt.expected {
				t.Errorf("findSafeStartPosition() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestFindSafeEndPosition(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		start    int
		expected int
	}{
		{
			name:     "complete end",
			input:    []byte("hello"),
			start:    0,
			expected: 5,
		},
		{
			name:     "incomplete end",
			input:    []byte{0xe4, 0xb8, 0x96, 0xe7, 0x95}, // "世" + 不完整的字符
			start:    0,
			expected: 3, // 应该截断到第一个字符后
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSafeEndPosition(tt.input, tt.start)
			if result != tt.expected {
				t.Errorf("findSafeEndPosition() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// 基准测试
func BenchmarkIsUTF8_SmallMemFile(b *testing.B) {
	data := []byte("hello 世界 ♥")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mf := memfile.New(data)
		IsUTF8(mf)
	}
}

func BenchmarkIsUTF8_LargeMemFile(b *testing.B) {
	data := []byte(strings.Repeat("hello 世界 ♥ 这是一个测试文件。", 1000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mf := memfile.New(data)
		IsUTF8(mf)
	}
}

func BenchmarkFixUTF8Boundaries(b *testing.B) {
	data := []byte("hello 世界 ♥")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fixUTF8Boundaries(data)
	}
}
