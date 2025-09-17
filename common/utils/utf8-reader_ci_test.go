package utils

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestUTF8Reader_CIStability 专门为CI环境设计的稳定性测试
func TestUTF8Reader_CIStability(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		bufSize   int
		chunkSize int
	}{
		{"ASCII", "Hello World", 5, 1},
		{"Chinese", "你好世界", 3, 1},
		{"Mixed", "Hello 世界", 6, 1},
		{"Emoji", "Hello 🌍", 8, 1},
		{"LargeBuffer", "Hello 世界 🌍", 100, 1},
		{"SmallBuffer", "Hello 世界", 2, 1},
		{"ChunkedInput", "测试文本", 10, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用不同的Reader实现来测试稳定性
			readers := []struct {
				name   string
				reader io.Reader
			}{
				{"BytewiseReader", &mockBytewiseReader{data: []byte(tt.input)}},
				{"ChunkedReader", &mockChunkedReader{data: []byte(tt.input), chunkSize: tt.chunkSize}},
			}

			for _, r := range readers {
				t.Run(r.name, func(t *testing.T) {
					utf8Reader := UTF8Reader(r.reader)

					// 读取所有数据
					result, err := io.ReadAll(utf8Reader)
					if err != nil {
						t.Fatalf("Failed to read from UTF8Reader: %v", err)
					}

					// 验证结果正确性
					if string(result) != tt.input {
						t.Errorf("Expected: %q, Got: %q", tt.input, string(result))
					}

					// 验证UTF-8有效性
					if !utf8.Valid(result) {
						t.Errorf("Result is not valid UTF-8: %v", result)
					}

					// 验证字符计数
					expectedRunes := utf8.RuneCountInString(tt.input)
					actualRunes := utf8.RuneCount(result)
					if expectedRunes != actualRunes {
						t.Errorf("Expected %d runes, got %d runes", expectedRunes, actualRunes)
					}
				})
			}
		})
	}
}

// TestUTF8Reader_EdgeCases 测试边界情况
func TestUTF8Reader_EdgeCases(t *testing.T) {
	t.Run("EmptyReader", func(t *testing.T) {
		reader := strings.NewReader("")
		utf8Reader := UTF8Reader(reader)

		buf := make([]byte, 10)
		n, err := utf8Reader.Read(buf)
		if n != 0 || err != io.EOF {
			t.Errorf("Expected (0, EOF), got (%d, %v)", n, err)
		}
	})

	t.Run("SingleByteUTF8", func(t *testing.T) {
		reader := &mockBytewiseReader{data: []byte("A")}
		utf8Reader := UTF8Reader(reader)

		result, err := io.ReadAll(utf8Reader)
		if err != nil {
			t.Fatalf("Failed to read: %v", err)
		}

		if string(result) != "A" {
			t.Errorf("Expected 'A', got %q", string(result))
		}
	})

	t.Run("MultiByteUTF8AtBoundary", func(t *testing.T) {
		// 测试3字节UTF-8字符在2字节边界处的情况
		text := "你" // 3字节字符
		reader := &mockChunkedReader{data: []byte(text), chunkSize: 2}
		utf8Reader := UTF8Reader(reader)

		result, err := io.ReadAll(utf8Reader)
		if err != nil {
			t.Fatalf("Failed to read: %v", err)
		}

		if string(result) != text {
			t.Errorf("Expected %q, got %q", text, string(result))
		}

		if !utf8.Valid(result) {
			t.Errorf("Result is not valid UTF-8")
		}
	})
}

// TestUTF8Reader_PerformanceStability 测试在不同性能环境下的稳定性
func TestUTF8Reader_PerformanceStability(t *testing.T) {
	// 创建较大的测试数据
	text := strings.Repeat("Hello 世界 🌍 测试 ", 100)

	// 测试不同的读取方式
	bufferSizes := []int{1, 3, 7, 16, 64, 256}

	for _, bufSize := range bufferSizes {
		t.Run(fmt.Sprintf("BufferSize%d", bufSize), func(t *testing.T) {
			reader := &mockBytewiseReader{data: []byte(text)}
			utf8Reader := UTF8Reader(reader)

			var result []byte
			buf := make([]byte, bufSize)

			for {
				n, err := utf8Reader.Read(buf)
				if n > 0 {
					result = append(result, buf[:n]...)

					// 在CI环境下，验证每次读取的内容都是有效的UTF-8
					// 除非是小缓冲区（1-3字节），这些情况下允许字符分割
					if bufSize >= 4 && !utf8.Valid(buf[:n]) {
						t.Errorf("Invalid UTF-8 in chunk with buffer size %d: %v", bufSize, buf[:n])
					}
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}

			if string(result) != text {
				t.Errorf("Final result mismatch with buffer size %d", bufSize)
			}

			if !utf8.Valid(result) {
				t.Errorf("Final result is not valid UTF-8 with buffer size %d", bufSize)
			}
		})
	}
}
