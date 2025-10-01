package ziputil

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestZip 创建测试用的 ZIP 文件
func createTestZip(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	for name, content := range files {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}

	err := w.Close()
	require.NoError(t, err)

	return buf.Bytes()
}

func TestGrepRegexp(t *testing.T) {
	// 创建测试数据
	files := map[string]string{
		"file1.txt": "Hello World\nThis is a test\nGoodbye",
		"file2.txt": "Another file\nWith some content\nTest123",
		"file3.log": "ERROR: something went wrong\nINFO: all good\nDEBUG: details",
	}

	zipData := createTestZip(t, files)

	// 测试正则表达式搜索
	t.Run("search with regexp", func(t *testing.T) {
		results, err := GrepRawRegexp(zipData, "test", WithGrepCaseSensitive())
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// 至少应该找到一个匹配
		found := false
		for _, r := range results {
			if strings.Contains(strings.ToLower(r.Line), "test") {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("search with limit", func(t *testing.T) {
		results, err := GrepRawRegexp(zipData, ".*", WithGrepLimit(2))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})

	t.Run("search ERROR pattern", func(t *testing.T) {
		results, err := GrepRawRegexp(zipData, "ERROR:.*")
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		found := false
		for _, r := range results {
			if strings.Contains(r.Line, "ERROR:") {
				assert.Equal(t, "file3.log", r.FileName)
				found = true
				break
			}
		}
		assert.True(t, found)
	})
}

func TestGrepSubString(t *testing.T) {
	files := map[string]string{
		"test1.txt": "Hello World\nTest Content\nGoodbye",
		"test2.txt": "Another TEST file\nWith content",
	}

	zipData := createTestZip(t, files)

	t.Run("case sensitive search", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "Test", WithGrepCaseSensitive())
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		for _, r := range results {
			assert.Contains(t, r.Line, "Test")
		}
	})

	t.Run("case insensitive search", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "test")
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})

	t.Run("search with context", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "Test", WithContext(1))
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// 检查是否有上下文
		for _, r := range results {
			if strings.Contains(r.Line, "Test") {
				// 可能有前置或后置上下文
				hasContext := len(r.ContextBefore) > 0 || len(r.ContextAfter) > 0
				if r.LineNumber > 1 || r.LineNumber < 3 {
					// 如果不是边界行，应该有上下文
					assert.True(t, hasContext, "should have context for line %d", r.LineNumber)
				}
			}
		}
	})
}

func TestExtractFile(t *testing.T) {
	files := map[string]string{
		"file1.txt":        "Content of file 1",
		"subdir/file2.txt": "Content of file 2",
		"file3.log":        "Log content",
	}

	zipData := createTestZip(t, files)

	t.Run("extract existing file", func(t *testing.T) {
		content, err := ExtractFileFromRaw(zipData, "file1.txt")
		require.NoError(t, err)
		assert.Equal(t, "Content of file 1", string(content))
	})

	t.Run("extract file in subdir", func(t *testing.T) {
		content, err := ExtractFileFromRaw(zipData, "subdir/file2.txt")
		require.NoError(t, err)
		assert.Equal(t, "Content of file 2", string(content))
	})

	t.Run("extract non-existing file", func(t *testing.T) {
		_, err := ExtractFileFromRaw(zipData, "nonexistent.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestExtractFiles(t *testing.T) {
	files := map[string]string{
		"file1.txt": "Content 1",
		"file2.txt": "Content 2",
		"file3.txt": "Content 3",
	}

	zipData := createTestZip(t, files)

	t.Run("extract multiple files", func(t *testing.T) {
		targets := []string{"file1.txt", "file2.txt"}
		results, err := ExtractFilesFromRaw(zipData, targets)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		// 验证内容
		contentMap := make(map[string]string)
		for _, r := range results {
			assert.NoError(t, r.Error)
			contentMap[r.FileName] = string(r.Content)
		}

		assert.Equal(t, "Content 1", contentMap["file1.txt"])
		assert.Equal(t, "Content 2", contentMap["file2.txt"])
	})

	t.Run("extract all files", func(t *testing.T) {
		targets := []string{"file1.txt", "file2.txt", "file3.txt"}
		results, err := ExtractFilesFromRaw(zipData, targets)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})
}

func TestExtractByPattern(t *testing.T) {
	files := map[string]string{
		"test1.txt":        "Content 1",
		"test2.txt":        "Content 2",
		"data.log":         "Log content",
		"config.json":      "{}",
		"subdir/test3.txt": "Content 3",
	}

	zipData := createTestZip(t, files)

	t.Run("extract by wildcard", func(t *testing.T) {
		results, err := ExtractByPatternFromRaw(zipData, "*.txt")
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// 所有结果应该以 .txt 结尾
		for _, r := range results {
			assert.True(t, strings.HasSuffix(r.FileName, ".txt"))
			assert.NoError(t, r.Error)
		}
	})

	t.Run("extract by prefix wildcard", func(t *testing.T) {
		results, err := ExtractByPatternFromRaw(zipData, "test*")
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// 所有结果应该以 test 开头
		for _, r := range results {
			// 可能是 test1.txt, test2.txt 或 subdir/test3.txt
			baseName := filepath.Base(r.FileName)
			assert.True(t, strings.HasPrefix(baseName, "test") || strings.HasPrefix(r.FileName, "test"))
		}
	})

	t.Run("extract all", func(t *testing.T) {
		results, err := ExtractByPatternFromRaw(zipData, "*")
		require.NoError(t, err)
		assert.Len(t, results, len(files))
	})
}

func TestCompressDecompress(t *testing.T) {
	// 创建临时目录
	tempDir := t.TempDir()

	// 创建测试文件
	testFile1 := filepath.Join(tempDir, "test1.txt")
	testFile2 := filepath.Join(tempDir, "test2.txt")

	err := os.WriteFile(testFile1, []byte("Test content 1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(testFile2, []byte("Test content 2"), 0644)
	require.NoError(t, err)

	// 压缩文件
	zipFile := filepath.Join(tempDir, "test.zip")
	err = CompressByName([]string{testFile1, testFile2}, zipFile)
	require.NoError(t, err)

	// 验证 ZIP 文件存在
	_, err = os.Stat(zipFile)
	require.NoError(t, err)

	// 解压缩
	extractDir := filepath.Join(tempDir, "extracted")
	err = DeCompress(zipFile, extractDir)
	require.NoError(t, err)

	// 验证解压后的文件
	extractedFile1 := filepath.Join(extractDir, testFile1)
	content1, err := os.ReadFile(extractedFile1)
	require.NoError(t, err)
	assert.Equal(t, "Test content 1", string(content1))
}

func TestGrepWithContext(t *testing.T) {
	files := map[string]string{
		"test.txt": `Line 1
Line 2
Target Line
Line 4
Line 5`,
	}

	zipData := createTestZip(t, files)

	results, err := GrepRawSubString(zipData, "Target", WithContext(2))
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	for _, r := range results {
		if strings.Contains(r.Line, "Target") {
			// 应该有前置上下文
			if r.LineNumber > 2 {
				assert.NotEmpty(t, r.ContextBefore, "should have context before for line %d", r.LineNumber)
			}
			// 应该有后置上下文
			assert.NotEmpty(t, r.ContextAfter, "should have context after")
		}
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		pattern  string
		expected bool
	}{
		{"exact match", "test.txt", "test.txt", true},
		{"suffix wildcard", "test.txt", "*.txt", true},
		{"prefix wildcard", "test.txt", "test*", true},
		{"middle wildcard", "test_file.txt", "test*.txt", true},
		{"no match", "test.log", "*.txt", false},
		{"match all", "anything", "*", true},
		{"complex pattern", "subdir/test.txt", "*test.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchPattern(tt.filename, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConcurrentExtraction(t *testing.T) {
	// 创建大量文件来测试并发
	files := make(map[string]string)
	for i := 0; i < 50; i++ {
		files[filepath.Join("file", "test_"+string(rune(i))+".txt")] = "Content " + string(rune(i))
	}

	zipData := createTestZip(t, files)

	// 提取所有文件
	var targets []string
	for name := range files {
		targets = append(targets, name)
	}

	results, err := ExtractFilesFromRaw(zipData, targets)
	require.NoError(t, err)
	assert.Len(t, results, len(files))

	// 验证所有文件都被正确提取
	for _, r := range results {
		assert.NoError(t, r.Error)
		assert.NotEmpty(t, r.Content)
	}
}
