package codec

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
)

func TestUTF8Safe(t *testing.T) {
	for _, c := range []struct {
		Input   string
		Contain []string
	}{
		{
			Input: "\x00E\x00S\x00你好", Contain: []string{"你好", "E", "S"},
		},
		{
			Input: "\xc4\xe3\xba\xc3", Contain: []string{`\xc4\xe3\xba\xc3`},
		},
	} {
		ret := UTF8SafeEscape(c.Input)
		log.Infof("UTF8SafeEscape(%#v) -> %#v", c.Input, ret)
		fmt.Println(ret)
		for _, s := range c.Contain {
			if !strings.Contains(ret, s) {
				t.Fatalf("expect: %#v in %#v", s, ret)
			}
		}
	}
}

func TestUTF8View(t *testing.T) {
	for _, c := range []struct {
		Input   string
		Contain []string
	}{
		{
			Input: "abcdefg\x00", Contain: []string{`abcdefg`},
		},
		{
			Input: "\u202eabc", Contain: []string{`abc`},
		},
		{
			Input: "\n\u202eabc", Contain: []string{"\nabc"},
		},
	} {
		ret := UTF8AndControlEscapeForEditorView(c.Input)
		log.Infof("UTF8SafeEscape(%#v) -> %#v", c.Input, ret)
		fmt.Println(ret)
		for _, s := range c.Contain {
			if !strings.Contains(ret, s) {
				t.Fatalf("expect: %#v in %#v", s, ret)
			}
		}
	}
}

func TestIsUTF8File(t *testing.T) {
	// Create temp directory for test files
	tempDir, err := ioutil.TempDir("", "utf8_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name     string
		content  []byte
		expected bool
		size     string // description of file size category
	}{
		{
			name:     "small_valid_utf8",
			content:  []byte("Hello, 世界! 这是一个测试文件。"),
			expected: true,
			size:     "small (<0.5K)",
		},
		{
			name:     "small_invalid_utf8",
			content:  []byte{0xFF, 0xFE, 0xFD, 0xFC}, // Invalid UTF-8 sequence
			expected: false,
			size:     "small (<0.5K)",
		},
		{
			name:     "small_ascii",
			content:  []byte("Hello, world! This is ASCII text."),
			expected: true,
			size:     "small (<0.5K)",
		},
		{
			name:     "medium_valid_utf8",
			content:  createMediumUTF8Content(),
			expected: true,
			size:     "medium (0.5K-1K)",
		},
		{
			name:     "medium_invalid_utf8",
			content:  createMediumInvalidContent(),
			expected: false,
			size:     "medium (0.5K-1K)",
		},
		{
			name:     "large_valid_utf8",
			content:  createLargeUTF8Content(),
			expected: true,
			size:     "large (>1K)",
		},
		{
			name:     "large_invalid_utf8",
			content:  createLargeInvalidContent(),
			expected: false,
			size:     "large (>1K)",
		},
		{
			name:     "empty_file",
			content:  []byte{},
			expected: true,
			size:     "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := filepath.Join(tempDir, tt.name+".txt")
			err := ioutil.WriteFile(filePath, tt.content, 0644)
			if err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Test the function
			result, err := IsUTF8File(filePath)
			if err != nil {
				t.Fatalf("IsUTF8File returned error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("IsUTF8File(%s) = %v, expected %v (size: %s, content length: %d)",
					tt.name, result, tt.expected, tt.size, len(tt.content))
			}

			log.Infof("Test %s (%s): content=%d bytes, result=%v",
				tt.name, tt.size, len(tt.content), result)
		})
	}
}

func TestIsUTF8FileError(t *testing.T) {
	// Test non-existent file
	_, err := IsUTF8File("/non/existent/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestUTF8BoundaryHandling(t *testing.T) {
	// Create temp directory for test files
	tempDir, err := ioutil.TempDir("", "utf8_boundary_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test file with UTF-8 characters that might be cut in the middle during sampling
	content := []byte("Hello ")
	// Add Chinese characters that use 3 bytes each in UTF-8
	chineseText := "这是一个很长的中文测试内容，用来测试UTF8边界处理功能。"
	content = append(content, []byte(chineseText)...)

	// Repeat to make it large enough for sampling
	for len(content) < 2000 {
		content = append(content, []byte(chineseText)...)
	}

	filePath := filepath.Join(tempDir, "boundary_test.txt")
	err = ioutil.WriteFile(filePath, content, 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := IsUTF8File(filePath)
	if err != nil {
		t.Fatalf("IsUTF8File returned error: %v", err)
	}

	if !result {
		t.Error("expected UTF-8 boundary handling to work correctly")
	}

	log.Infof("Boundary test passed: file with %d bytes correctly identified as UTF-8", len(content))
}

// Helper functions to create test content

func createMediumUTF8Content() []byte {
	base := "这是一个中等大小的UTF-8测试文件。包含中文字符、English text、和各种符号！@#$%^&*()_+"
	content := []byte(base)

	// Repeat until we get to medium size (0.5K-1K)
	for len(content) < 800 {
		content = append(content, []byte(base)...)
	}

	// Trim to exactly 800 bytes, but make sure we don't cut in the middle of a UTF-8 character
	if len(content) > 800 {
		content = content[:800]
		// Find the last valid UTF-8 boundary
		for i := len(content) - 1; i >= 0; i-- {
			if utf8.Valid(content[:i+1]) {
				content = content[:i+1]
				break
			}
		}
	}

	return content
}

func createMediumInvalidContent() []byte {
	content := createMediumUTF8Content()
	// Insert invalid UTF-8 bytes in positions that will be sampled
	// For medium files, we sample the first 512 bytes, so put invalid bytes early
	positions := []int{100, 200, 300}
	invalidBytes := []byte{0xFF, 0xFE, 0xFD}

	for i, pos := range positions {
		if pos < len(content) {
			content[pos] = invalidBytes[i]
		}
	}
	return content
}

func createLargeUTF8Content() []byte {
	base := "这是一个大型UTF-8测试文件内容。包含中文、English、数字123、特殊符号！@#$%^&*()_+=[]{};':\",./<>?\n"
	content := []byte(base)

	// Repeat until we get to large size (>1K)
	for len(content) < 3000 {
		content = append(content, []byte(base)...)
	}

	return content
}

func createLargeInvalidContent() []byte {
	content := createLargeUTF8Content()
	// Insert invalid UTF-8 bytes at multiple positions to ensure they appear in samples
	// Large files use multiple samples, so distribute invalid bytes across different regions
	positions := []int{200, 600, 1200, 1800, 2400}
	invalidBytes := []byte{0xFF, 0xFE, 0xFD, 0xFC}

	for i, pos := range positions {
		if pos < len(content) {
			content[pos] = invalidBytes[i%len(invalidBytes)]
		}
	}

	return content
}
