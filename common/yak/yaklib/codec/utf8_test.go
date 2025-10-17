package codec

import (
	"os"
	"path/filepath"
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

func TestFixUTF8EndBoundary(t *testing.T) {
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
			name:     "incomplete at end",
			input:    []byte{0xe4, 0xb8, 0x96, 0xe7, 0x95, 0x8c, 0xe4, 0xb8}, // "世界" + 不完整的字符
			expected: "世界",
		},
		{
			name:     "normal ascii",
			input:    []byte("hello"),
			expected: "hello",
		},
		{
			name:     "valid with middle invalid bytes",                                        // 这个应该保持原样，不修复中间的无效字节
			input:    []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0xff, 0x77, 0x6f, 0x72, 0x6c, 0x64}, // "hello" + 0xff + "world"
			expected: string([]byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0xff, 0x77, 0x6f, 0x72, 0x6c, 0x64}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixUTF8EndBoundary(tt.input)
			if string(result) != tt.expected {
				t.Errorf("fixUTF8EndBoundary() = %q, want %q", string(result), tt.expected)
			}
		})
	}
}

func BenchmarkFixUTF8Boundaries(b *testing.B) {
	data := []byte("hello 世界 ♥")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fixUTF8Boundaries(data)
	}
}

func TestIsUTF8_BinaryData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "PNG header",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG signature
			expected: false,
		},
		{
			name:     "JPEG header",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG signature
			expected: false,
		},
		{
			name:     "GIF header (ASCII but binary context)",
			data:     []byte{0x47, 0x49, 0x46, 0x38, 0x37, 0x61}, // GIF87a - ASCII字符，UTF-8有效
			expected: true,                                       // 技术上是有效UTF-8
		},
		{
			name:     "PDF header (ASCII but binary context)",
			data:     []byte{0x25, 0x50, 0x44, 0x46, 0x2D}, // %PDF- - ASCII字符，UTF-8有效
			expected: true,                                 // 技术上是有效UTF-8
		},
		{
			name:     "Executable (PE) header",
			data:     []byte{0x4D, 0x5A, 0x90, 0x00, 0x03, 0x00}, // MZ header - 包含非UTF-8字节
			expected: false,
		},
		{
			name:     "ZIP header (ASCII but binary context)",
			data:     []byte{0x50, 0x4B, 0x03, 0x04}, // PK signature - 包含控制字符
			expected: true,                           // 技术上是有效UTF-8（控制字符是有效的）
		},
		{
			name:     "Random binary data",
			data:     []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC},
			expected: false,
		},
		{
			name:     "High-bit bytes",
			data:     []byte{0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87},
			expected: false,
		},
		{
			name:     "Null bytes mixed with text",
			data:     []byte{'h', 'e', 'l', 'l', 'o', 0x00, 'w', 'o', 'r', 'l', 'd'},
			expected: true, // null字节在UTF-8中是有效的
		},
		{
			name:     "Control characters",
			data:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			expected: true, // 控制字符在UTF-8中是有效的
		},
		{
			name:     "Invalid UTF-8 sequence",
			data:     []byte{0xC0, 0x80}, // 无效的UTF-8序列
			expected: false,
		},
		{
			name:     "Incomplete UTF-8 sequence",
			data:     []byte{0xE4, 0xB8}, // 不完整的UTF-8序列
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with memfile
			mf := memfile.New(tt.data)
			result, err := IsUTF8(mf)
			if err != nil {
				t.Fatalf("IsUTF8() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("IsUTF8() = %v, want %v for %s", result, tt.expected, tt.name)
			}

			// Test with bytes
			result2, err := IsUTF8(tt.data)
			if err != nil {
				t.Fatalf("IsUTF8() error = %v", err)
			}
			if result2 != tt.expected {
				t.Errorf("IsUTF8() bytes = %v, want %v for %s", result2, tt.expected, tt.name)
			}
		})
	}
}

func TestIsUTF8File_BinaryFiles(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "utf8_binary_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "fake_image.png",
			data:     createFakePNGData(),
			expected: false,
		},
		{
			name:     "fake_jpeg.jpg",
			data:     createFakeJPEGData(),
			expected: false,
		},
		{
			name:     "binary_executable",
			data:     createFakeBinaryData(),
			expected: false,
		},
		{
			name:     "mixed_utf8_binary",
			data:     createMixedUTF8BinaryData(),
			expected: false,
		},
		{
			name: "mostly_ascii_with_nulls_and_high_bytes",
			data: append([]byte("This is mostly ASCII text\x00but has null bytes\x00scattered throughout"),
				[]byte{0x80, 0x81, 0x82, 0x83, 0xFF, 0xFE, 0xFD, 0xFC, 0x90, 0x91, 0x92, 0x93}...),
			expected: false,
		},
		{
			name:     "valid_utf8_with_newlines",
			data:     []byte("这是一个包含\n换行符的\n有效UTF-8文本\n测试文件"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := filepath.Join(tempDir, tt.name)
			err := os.WriteFile(filePath, tt.data, 0644)
			if err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Test the function
			result, err := IsUTF8File(filePath)
			if err != nil {
				t.Fatalf("IsUTF8File returned error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("IsUTF8File(%s) = %v, want %v (size: %d bytes)",
					tt.name, result, tt.expected, len(tt.data))
			}

			t.Logf("Test %s: %d bytes, result=%v", tt.name, len(tt.data), result)
		})
	}
}

// Helper functions to create fake binary data
func createFakePNGData() []byte {
	// PNG signature + fake IHDR chunk
	data := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature (包含0x89)
		0x00, 0x00, 0x00, 0x0D, // IHDR chunk length
		0x49, 0x48, 0x44, 0x52, // IHDR
		0x00, 0x00, 0x00, 0x20, // Width: 32
		0x00, 0x00, 0x00, 0x20, // Height: 32
		0x08, 0x02, 0x00, 0x00, 0x00, // bit depth, color type, etc.
	}

	// 立即添加大量无效UTF-8字节，确保采样会遇到它们
	for i := 0; i < 100; i++ {
		data = append(data, byte(0x80+i%128)) // 连续的无效UTF-8字节
	}

	// Add more binary data with many bytes > 0x7F
	for i := 0; i < 900; i++ {
		// 确保有很多大于0x7F的字节
		if i%3 == 0 {
			data = append(data, byte(0x80+i%128)) // 0x80-0xFF范围
		} else if i%3 == 1 {
			data = append(data, byte(0x90+i%100)) // 高位字节
		} else {
			data = append(data, byte(0xC0+i%64)) // 无效UTF-8起始字节
		}
	}
	return data
}

func createFakeJPEGData() []byte {
	// JPEG signature + fake data
	data := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, // JPEG signature (包含0xFF)
		0x00, 0x10, // APP0 length
		0x4A, 0x46, 0x49, 0x46, 0x00, // JFIF\0
		0x01, 0x01, // version
		0x01, 0x00, 0x01, 0x00, 0x01, // units, density
		0x00, 0x00, // thumbnail
	}
	// Add binary data with lots of high bytes
	for i := 0; i < 800; i++ {
		// 大量高位字节，确保无效UTF-8
		if i%2 == 0 {
			data = append(data, byte(0x80+i%128))
		} else {
			data = append(data, byte(0xA0+i%96))
		}
	}
	return data
}

func createFakeBinaryData() []byte {
	// Simulate executable with PE header
	data := []byte{
		0x4D, 0x5A, // MZ signature
		0x90, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04, 0x00,
		0x00, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0xB8, 0x00,
	}

	// 立即添加大量无效UTF-8字节在开头，确保任何采样都会遇到
	for i := 0; i < 200; i++ {
		data = append(data, byte(0x80+i%128)) // 连续无效字节
	}

	// Add lots of binary data with high bytes
	for i := 0; i < 1800; i++ {
		// 混合各种高位字节，确保无效UTF-8
		if i%4 == 0 {
			data = append(data, byte(0x80+i%128))
		} else if i%4 == 1 {
			data = append(data, byte(0xC0+i%64)) // 无效UTF-8起始字节
		} else if i%4 == 2 {
			data = append(data, byte(0xE0+i%32)) // 无效UTF-8起始字节
		} else {
			data = append(data, byte(0xF8+i%8)) // 完全无效的字节
		}
	}
	return data
}

func createMixedUTF8BinaryData() []byte {
	// Start with valid UTF-8
	data := []byte("这是一些UTF-8文本")

	// Add some binary data with high bytes
	binaryChunk := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC, 0x80, 0x90, 0xA0, 0xB0}
	data = append(data, binaryChunk...)

	// Add more UTF-8
	data = append(data, []byte("更多的中文文本")...)

	// Add more binary with lots of high bytes
	for i := 0; i < 100; i++ {
		// 确保有大量无效UTF-8字节
		if i%3 == 0 {
			data = append(data, byte(0x80+i%128))
		} else if i%3 == 1 {
			data = append(data, byte(0xC0+i%64)) // 无效的UTF-8起始字节
		} else {
			data = append(data, byte(0xF8+i%8)) // 无效的UTF-8字节
		}
	}

	return data
}

func BenchmarkFixUTF8EndBoundary(b *testing.B) {
	data := []byte("hello 世界 ♥")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fixUTF8EndBoundary(data)
	}
}

func TestFixUTF8SampleBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "complete utf8",
			input:    []byte("hello 世界"),
			expected: []byte("hello 世界"),
		},
		{
			name:     "empty input",
			input:    []byte(""),
			expected: []byte(""),
		},
		{
			name:     "incomplete at end",
			input:    []byte{0xe4, 0xb8, 0x96, 0xe7, 0x95, 0x8c, 0xe4, 0xb8}, // "世界" + 不完整的字符
			expected: []byte{0xe4, 0xb8, 0x96, 0xe7, 0x95, 0x8c},             // 应该截断不完整的字符
		},
		{
			name:     "invalid bytes in middle should be preserved",
			input:    []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0xff, 0x77, 0x6f, 0x72, 0x6c, 0x64}, // "hello" + 0xff + "world"
			expected: []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0xff, 0x77, 0x6f, 0x72, 0x6c, 0x64}, // 应该保持原样
		},
		{
			name:     "binary data with PNG signature",
			input:    []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}, // PNG开头
			expected: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}, // 应该保持原样
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixUTF8SampleBoundaries(tt.input)
			if !equalBytes(result, tt.expected) {
				t.Errorf("fixUTF8SampleBoundaries() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func BenchmarkIsUTF8_BinaryData(b *testing.B) {
	data := createFakePNGData()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mf := memfile.New(data)
		IsUTF8(mf)
	}
}

func BenchmarkFixUTF8SampleBoundaries(b *testing.B) {
	data := []byte("hello 世界 ♥")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fixUTF8SampleBoundaries(data)
	}
}
