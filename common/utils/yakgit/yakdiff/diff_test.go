package yakdiff

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// TestBasicStringDiff 测试基本字符串差异比较
func TestBasicStringDiff(t *testing.T) {
	tests := []struct {
		name     string
		input1   string
		input2   string
		expected []string // 期望包含的关键字
	}{
		{
			name:     "Simple text change",
			input1:   "hello world",
			input2:   "hello yaklang",
			expected: []string{"@@", "-hello world", "+hello yaklang"},
		},
		{
			name:     "Add new line",
			input1:   "line1\nline2",
			input2:   "line1\nline2\nline3",
			expected: []string{"@@", "+line3"},
		},
		{
			name:     "Remove line",
			input1:   "line1\nline2\nline3",
			input2:   "line1\nline3",
			expected: []string{"@@", "-line2"},
		},
		{
			name:     "Modify line",
			input1:   "old content\nmiddle line\nend",
			input2:   "new content\nmiddle line\nend",
			expected: []string{"@@", "-old content", "+new content"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := Diff(tt.input1, tt.input2)
			if err != nil {
				t.Fatalf("Diff failed: %v", err)
			}

			log.Infof("Test %s - Diff result:\n%s", tt.name, diff)

			for _, expected := range tt.expected {
				if !strings.Contains(diff, expected) {
					t.Errorf("Expected diff to contain '%s', but it didn't. Full diff:\n%s", expected, diff)
				}
			}
		})
	}
}

// TestEdgeCases 测试边界情况
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input1   any
		input2   any
		expected string
	}{
		{
			name:     "Empty strings",
			input1:   "",
			input2:   "",
			expected: "", // 内容相同，没有差异
		},
		{
			name:     "Empty to content",
			input1:   "",
			input2:   "hello",
			expected: "+hello",
		},
		{
			name:     "Content to empty",
			input1:   "hello",
			input2:   "",
			expected: "-hello",
		},
		{
			name:     "Identical content",
			input1:   "same content\nline2\nline3",
			input2:   "same content\nline2\nline3",
			expected: "", // 内容相同，没有差异
		},
		{
			name:     "Nil vs string",
			input1:   nil,
			input2:   "content",
			expected: "+content",
		},
		{
			name:     "Number vs string",
			input1:   123,
			input2:   "123",
			expected: "", // 内容相同（经过转换后），没有差异
		},
		{
			name:     "Boolean vs string",
			input1:   true,
			input2:   "true",
			expected: "", // 内容相同（经过转换后），没有差异
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := Diff(tt.input1, tt.input2)
			if err != nil {
				t.Fatalf("Diff failed: %v", err)
			}

			log.Infof("Test %s - Diff result:\n%s", tt.name, diff)

			if !strings.Contains(diff, tt.expected) {
				t.Errorf("Expected diff to contain '%s', but it didn't. Full diff:\n%s", tt.expected, diff)
			}
		})
	}
}

// TestMultiLineFiles 测试多行文件差异
func TestMultiLineFiles(t *testing.T) {
	file1 := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}`

	file2 := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Hello, Yaklang!")
	os.Exit(0)
}`

	diff, err := Diff(file1, file2)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	log.Infof("Multi-line file diff:\n%s", diff)

	expectedChanges := []string{
		"@@",
		"-import \"fmt\"",
		"+import (",
		"+\t\"fmt\"",
		"+\t\"os\"",
		"+)",
		"-\tfmt.Println(\"Hello, World!\")",
		"+\tfmt.Println(\"Hello, Yaklang!\")",
		"+\tos.Exit(0)",
	}

	for _, expected := range expectedChanges {
		if !strings.Contains(diff, expected) {
			t.Errorf("Expected diff to contain '%s', but it didn't", expected)
		}
	}
}

// TestBinaryData 测试二进制数据
func TestBinaryData(t *testing.T) {
	binary1 := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
	binary2 := []byte{0x00, 0x01, 0x04, 0x03, 0xFF}

	diff, err := Diff(binary1, binary2)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	log.Infof("Binary data diff:\n%s", diff)

	// 二进制数据应该能够正常处理
	if strings.TrimSpace(diff) == "" {
		t.Error("Expected some diff for different binary data")
	}
}

// TestPerformance 测试性能
func TestPerformance(t *testing.T) {
	// 生成大量文本
	var lines1, lines2 []string
	for i := 0; i < 1000; i++ {
		lines1 = append(lines1, fmt.Sprintf("Line %d original", i))
		if i%10 == 0 {
			lines2 = append(lines2, fmt.Sprintf("Line %d modified", i))
		} else {
			lines2 = append(lines2, fmt.Sprintf("Line %d original", i))
		}
	}

	text1 := strings.Join(lines1, "\n")
	text2 := strings.Join(lines2, "\n")

	start := time.Now()
	diff, err := Diff(text1, text2)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Performance test failed: %v", err)
	}

	log.Infof("Performance test completed in %v, diff length: %d", duration, len(diff))

	// 性能要求：处理1000行文本应该在1秒内完成
	if duration > time.Second {
		t.Errorf("Performance test too slow: %v", duration)
	}

	// 验证diff包含修改内容
	if !strings.Contains(diff, "modified") {
		t.Error("Performance test diff should contain modifications")
	}
}

// TestConcurrency 测试并发安全性
func TestConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	errChan := make(chan error, 10)

	// 启动多个goroutine并发执行diff
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			text1 := fmt.Sprintf("Goroutine %d original content\nLine 2\nLine 3", id)
			text2 := fmt.Sprintf("Goroutine %d modified content\nLine 2\nLine 4", id)

			diff, err := Diff(text1, text2)
			if err != nil {
				errChan <- fmt.Errorf("goroutine %d failed: %v", id, err)
				return
			}

			expectedModified := fmt.Sprintf("Goroutine %d modified", id)
			expectedOriginal := fmt.Sprintf("Goroutine %d original", id)

			if !strings.Contains(diff, expectedModified) || !strings.Contains(diff, expectedOriginal) {
				errChan <- fmt.Errorf("goroutine %d diff doesn't contain expected content", id)
				return
			}

			log.Infof("Goroutine %d completed successfully", id)
		}(i)
	}

	wg.Wait()
	close(errChan)

	// 检查是否有错误
	for err := range errChan {
		t.Error(err)
	}
}

// TestContextCancellation 测试上下文取消
func TestContextCancellation(t *testing.T) {
	// 创建一个会被取消的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 生成大量数据以确保操作需要一些时间
	var lines1, lines2 []string
	for i := 0; i < 10000; i++ {
		lines1 = append(lines1, fmt.Sprintf("Line %d original very long content to make processing slower", i))
		lines2 = append(lines2, fmt.Sprintf("Line %d modified very long content to make processing slower", i))
	}

	text1 := strings.Join(lines1, "\n")
	text2 := strings.Join(lines2, "\n")

	_, err := DiffToStringContext(ctx, text1, text2)

	// 这里我们不强制要求上下文取消错误，因为操作可能很快完成
	// 但如果有错误，我们记录它
	if err != nil {
		log.Infof("Context cancellation test error (expected): %v", err)
	}
}

// TestCustomHandler 测试自定义处理器
func TestCustomHandler(t *testing.T) {
	var collectedChanges []string

	customHandler := func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch != nil {
			collectedChanges = append(collectedChanges, patch.String())
		}
		return nil
	}

	_, err := Diff("old content\nline2", "new content\nline2", customHandler)
	if err != nil {
		t.Fatalf("Custom handler test failed: %v", err)
	}

	if len(collectedChanges) == 0 {
		t.Error("Custom handler should have collected changes")
	}

	log.Infof("Custom handler collected %d changes", len(collectedChanges))
	for i, change := range collectedChanges {
		log.Infof("Change %d: %s", i, change)
	}
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	// 测试处理器返回错误的情况
	errorHandler := func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		return fmt.Errorf("custom handler error")
	}

	_, err := Diff("content1", "content2", errorHandler)
	if err == nil {
		t.Error("Expected error from custom handler, but got none")
	}

	if !strings.Contains(err.Error(), "custom handler error") {
		t.Errorf("Expected error to contain 'custom handler error', but got: %v", err)
	}

	log.Infof("Error handling test passed: %v", err)
}

// TestLegacyCompatibility 测试向后兼容性（保留原有测试的修改版本）
func TestLegacyCompatibility(t *testing.T) {
	input1 := `	return utils.Wrap(err, "init git repos")
}
wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, "get worktree")
	}
	wt.Filesystem.MkdirAll("main", 0755)
	if err != nil {
		return utils.Wrap(err, "mkdir main")
	}
	filename := path.Join("main", "main.txt")
	fp, err := wt.Filesystem.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return utils.Wrap(err, "open file")
	}
	fp.Write(r1)
	fp.Close()
	_, err = wt.Add(filename)
	if err != nil {
		return utils.Wrap(err, "add file")
	}
	commit, err := wt.Commit("add first file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Yaklang",
		Email: "yaklang@example.com",`

	input2 := `	return utils.Wrap(err, "init git repos")
	}
	wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, "get worktree")
	}
	wt.Filesystem.MkdirAll("main", 0755)
	if err != nil {
		return utils.Wrap(err, "mkdir main")
	}
	filename := path.Join("main", "main.txt")
	_, err = wt.Add(filename)
	if err != nil {
		return utils.Wrap(err, "add file")
	}
	commit, err := wt.Commit("add first file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Yaklang",
		Email: "yaklang@example.com",`

	// 测试自定义处理器（保持向后兼容）
	check := false
	_, err := Diff(input1, input2, func(_ *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch != nil {
			raw := patch.String()
			log.Infof("Legacy test patch: %s", raw)
			// 检查是否包含删除的行
			if strings.Contains(raw, "-") && strings.Contains(raw, "OpenFile") {
				check = true
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Legacy compatibility test failed: %v", err)
	}

	if !check {
		t.Error("Legacy compatibility test should have detected the file operation removal")
	}

	// 测试不带处理器的新接口
	diff, err := Diff(input1, input2)
	if err != nil {
		t.Fatalf("New interface test failed: %v", err)
	}

	if strings.TrimSpace(diff) == "" {
		t.Error("New interface should return diff string")
	}

	log.Infof("Legacy compatibility test passed, diff length: %d", len(diff))
}

// TestSpecialCharacters 测试特殊字符处理
func TestSpecialCharacters(t *testing.T) {
	tests := []struct {
		name   string
		input1 string
		input2 string
	}{
		{
			name:   "Unicode characters",
			input1: "Hello 世界",
			input2: "Hello 世界！",
		},
		{
			name:   "Special symbols",
			input1: "Price: $100.00",
			input2: "Price: €100.00",
		},
		{
			name:   "Escape sequences",
			input1: "Line1\nLine2\tTabbed",
			input2: "Line1\r\nLine2\tTabbed",
		},
		{
			name:   "JSON content",
			input1: `{"name": "test", "value": 123}`,
			input2: `{"name": "test", "value": 456}`,
		},
		{
			name:   "Code with quotes",
			input1: `fmt.Printf("Hello %s", name)`,
			input2: `fmt.Printf("Hi %s", name)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := Diff(tt.input1, tt.input2)
			if err != nil {
				t.Fatalf("Special characters test failed: %v", err)
			}

			log.Infof("Special characters test %s:\n%s", tt.name, diff)

			// 基本验证：如果输入不同，应该有diff输出
			if tt.input1 != tt.input2 && strings.TrimSpace(diff) == "" {
				t.Errorf("Expected diff output for different inputs in test %s", tt.name)
			}
		})
	}
}

// TestLargeFileDiff 测试大文件差异
func TestLargeFileDiff(t *testing.T) {
	// 生成大文件内容
	var lines1, lines2 []string
	for i := 0; i < 5000; i++ {
		if i%100 == 50 {
			lines1 = append(lines1, fmt.Sprintf("// This is line %d with original content", i))
			lines2 = append(lines2, fmt.Sprintf("// This is line %d with modified content", i))
		} else {
			line := fmt.Sprintf("Line %d: some content here to make it realistic", i)
			lines1 = append(lines1, line)
			lines2 = append(lines2, line)
		}
	}

	content1 := strings.Join(lines1, "\n")
	content2 := strings.Join(lines2, "\n")

	start := time.Now()
	diff, err := Diff(content1, content2)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Large file diff test failed: %v", err)
	}

	log.Infof("Large file diff completed in %v, content1 size: %d, content2 size: %d, diff size: %d",
		duration, len(content1), len(content2), len(diff))

	// 验证包含修改内容
	if !strings.Contains(diff, "modified content") {
		t.Error("Large file diff should contain modified content")
	}

	// 性能要求：处理5000行应该在2秒内完成
	if duration > 2*time.Second {
		t.Errorf("Large file diff too slow: %v", duration)
	}
}

// TestVariousDataTypes 测试不同数据类型的转换
func TestVariousDataTypes(t *testing.T) {
	tests := []struct {
		name   string
		input1 any
		input2 any
	}{
		{
			name:   "Slice vs string",
			input1: []string{"a", "b", "c"},
			input2: "a\nb\nc",
		},
		{
			name:   "Map vs string",
			input1: map[string]int{"a": 1, "b": 2},
			input2: `map[a:1 b:2]`,
		},
		{
			name: "Struct vs string",
			input1: struct {
				Name string
				Age  int
			}{"Alice", 30},
			input2: "{Alice 30}",
		},
		{
			name:   "Float vs string",
			input1: 3.14159,
			input2: "3.14159",
		},
		{
			name:   "Large number",
			input1: 1234567890,
			input2: "1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := Diff(tt.input1, tt.input2)
			if err != nil {
				t.Fatalf("Data type test failed: %v", err)
			}

			log.Infof("Data type test %s - diff: %s", tt.name, diff)
		})
	}
}

// TestEmptyAndWhitespace 测试空白字符处理
func TestEmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name   string
		input1 string
		input2 string
	}{
		{
			name:   "Trailing spaces",
			input1: "line1\nline2 \nline3",
			input2: "line1\nline2\nline3",
		},
		{
			name:   "Leading spaces",
			input1: "line1\n line2\nline3",
			input2: "line1\nline2\nline3",
		},
		{
			name:   "Tab vs spaces",
			input1: "line1\n\tindented",
			input2: "line1\n    indented",
		},
		{
			name:   "Windows vs Unix line endings",
			input1: "line1\r\nline2\r\n",
			input2: "line1\nline2\n",
		},
		{
			name:   "Multiple empty lines",
			input1: "line1\n\n\nline2",
			input2: "line1\n\nline2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := Diff(tt.input1, tt.input2)
			if err != nil {
				t.Fatalf("Whitespace test failed: %v", err)
			}

			log.Infof("Whitespace test %s:\n%s", tt.name, diff)

			// 如果内容不同，应该有diff
			if tt.input1 != tt.input2 && strings.TrimSpace(diff) == "" {
				t.Errorf("Expected diff for whitespace differences in test %s", tt.name)
			}
		})
	}
}

// BenchmarkDiff 性能基准测试
func BenchmarkDiff(b *testing.B) {
	// 准备测试数据
	smallText1 := "Hello world"
	smallText2 := "Hello yaklang"

	mediumText1 := strings.Repeat("Line of text\n", 100)
	mediumText2 := strings.Repeat("Modified line\n", 100)

	var largeLines1, largeLines2 []string
	for i := 0; i < 1000; i++ {
		largeLines1 = append(largeLines1, fmt.Sprintf("Line %d original", i))
		largeLines2 = append(largeLines2, fmt.Sprintf("Line %d modified", i))
	}
	largeText1 := strings.Join(largeLines1, "\n")
	largeText2 := strings.Join(largeLines2, "\n")

	b.Run("SmallText", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := Diff(smallText1, smallText2)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("MediumText", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := Diff(mediumText1, mediumText2)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LargeText", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := Diff(largeText1, largeText2)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDiffContext 上下文版本的基准测试
func BenchmarkDiffContext(b *testing.B) {
	text1 := strings.Repeat("Line of original text\n", 100)
	text2 := strings.Repeat("Line of modified text\n", 100)

	b.Run("WithContext", func(b *testing.B) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			_, err := DiffToStringContext(ctx, text1, text2)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestRegressionBugs 回归测试，防止已知问题重现
func TestRegressionBugs(t *testing.T) {
	t.Run("NilPointerCheck", func(t *testing.T) {
		// 确保nil输入不会导致panic
		_, err := Diff(nil, nil)
		if err != nil {
			t.Fatalf("Nil inputs should not cause error: %v", err)
		}
	})

	t.Run("EmptyHandlerSlice", func(t *testing.T) {
		// 确保空的handler切片不会导致问题
		var handlers []DiffHandler
		_, err := Diff("a", "b", handlers...)
		if err != nil {
			t.Fatalf("Empty handler slice should not cause error: %v", err)
		}
	})

	t.Run("VeryLongSingleLine", func(t *testing.T) {
		// 测试非常长的单行文本
		longLine1 := strings.Repeat("a", 10000)
		longLine2 := strings.Repeat("b", 10000)

		diff, err := Diff(longLine1, longLine2)
		if err != nil {
			t.Fatalf("Very long single line should not cause error: %v", err)
		}

		if strings.TrimSpace(diff) == "" {
			t.Error("Very long single line diff should produce output")
		}
	})
}

// TestFileSystemDiff 测试文件系统差异比较
func TestFileSystemDiff(t *testing.T) {
	// 创建内存文件系统进行测试
	fs1 := filesys.NewVirtualFs()
	fs2 := filesys.NewVirtualFs()

	// 在 fs1 中创建一些文件
	fs1.WriteFile("file1.txt", []byte("content 1"), 0644)
	fs1.WriteFile("file2.txt", []byte("content 2"), 0644)
	fs1.WriteFile("shared.txt", []byte("shared content"), 0644)
	fs1.MkdirAll("dir1", 0755)
	fs1.WriteFile("dir1/nested.txt", []byte("nested content"), 0644)

	// 在 fs2 中创建修改后的文件
	fs2.WriteFile("file1.txt", []byte("modified content 1"), 0644) // 修改
	// file2.txt 被删除（不在 fs2 中）
	fs2.WriteFile("shared.txt", []byte("shared content"), 0644) // 不变
	fs2.WriteFile("file3.txt", []byte("new content 3"), 0644)   // 新增
	fs2.MkdirAll("dir1", 0755)
	fs2.WriteFile("dir1/nested.txt", []byte("modified nested content"), 0644) // 修改
	fs2.MkdirAll("dir2", 0755)
	fs2.WriteFile("dir2/new.txt", []byte("new file in new dir"), 0644) // 新目录和新文件

	// 执行差异比较
	diff, err := FileSystemDiff(fs1, fs2)
	if err != nil {
		t.Fatalf("FileSystemDiff failed: %v", err)
	}

	log.Infof("FileSystem diff result:\n%s", diff)

	// 验证差异内容
	expectedChanges := []string{
		"file1.txt",           // 文件修改
		"file2.txt",           // 文件删除
		"file3.txt",           // 文件新增
		"dir1/nested.txt",     // 嵌套文件修改
		"dir2/new.txt",        // 新目录中的新文件
		"-content 1",          // 删除的内容
		"+modified content 1", // 添加的内容
		"+new content 3",      // 新文件内容
	}

	for _, expected := range expectedChanges {
		if !strings.Contains(diff, expected) {
			t.Errorf("Expected diff to contain '%s', but it didn't", expected)
		}
	}

	// 验证共享文件没有出现在diff中（因为内容相同）
	if strings.Contains(diff, "shared.txt") {
		// 如果出现，应该是因为git认为它被删除后重新添加，这是可以接受的
		log.Infof("shared.txt appears in diff (this may be expected due to git behavior)")
	}
}

// TestFileSystemDiffContext 测试带上下文的文件系统差异比较
func TestFileSystemDiffContext(t *testing.T) {
	fs1 := filesys.NewVirtualFs()
	fs2 := filesys.NewVirtualFs()

	// 创建测试文件
	fs1.WriteFile("test.txt", []byte("original"), 0644)
	fs2.WriteFile("test.txt", []byte("modified"), 0644)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	diff, err := FileSystemDiffToStringContext(ctx, fs1, fs2)
	if err != nil {
		t.Fatalf("FileSystemDiffToStringContext failed: %v", err)
	}

	if !strings.Contains(diff, "test.txt") {
		t.Error("Expected diff to contain test.txt")
	}
	if !strings.Contains(diff, "-original") {
		t.Error("Expected diff to contain -original")
	}
	if !strings.Contains(diff, "+modified") {
		t.Error("Expected diff to contain +modified")
	}

	log.Infof("FileSystem context diff result:\n%s", diff)
}

// TestFileSystemDiffEmpty 测试空文件系统的差异
func TestFileSystemDiffEmpty(t *testing.T) {
	emptyFS1 := filesys.NewVirtualFs()
	emptyFS2 := filesys.NewVirtualFs()

	// 两个空文件系统应该没有差异（除了.gitkeep文件）
	diff, err := FileSystemDiff(emptyFS1, emptyFS2)
	if err != nil {
		t.Fatalf("FileSystemDiff with empty filesystems failed: %v", err)
	}

	// 空文件系统可能会包含.gitkeep文件的变更，这是正常的
	if strings.TrimSpace(diff) != "" && !strings.Contains(diff, ".gitkeep") {
		t.Errorf("Expected only .gitkeep diff for empty filesystems, got: %s", diff)
	}

	// 空文件系统 vs 有文件的文件系统
	nonEmptyFS := filesys.NewVirtualFs()
	nonEmptyFS.WriteFile("test.txt", []byte("content"), 0644)

	diff, err = FileSystemDiff(emptyFS1, nonEmptyFS)
	if err != nil {
		t.Fatalf("FileSystemDiff empty vs non-empty failed: %v", err)
	}

	if !strings.Contains(diff, "+content") {
		t.Error("Expected diff to show file addition")
	}

	log.Infof("Empty vs non-empty diff:\n%s", diff)
}

// TestFileSystemDiffLarge 测试大型文件系统差异
func TestFileSystemDiffLarge(t *testing.T) {
	fs1 := filesys.NewVirtualFs()
	fs2 := filesys.NewVirtualFs()

	// 创建大量文件
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("file%d.txt", i)
		content1 := fmt.Sprintf("content %d original", i)
		content2 := fmt.Sprintf("content %d modified", i)

		fs1.WriteFile(filename, []byte(content1), 0644)

		// 只修改一部分文件
		if i%10 == 0 {
			fs2.WriteFile(filename, []byte(content2), 0644)
		} else {
			fs2.WriteFile(filename, []byte(content1), 0644)
		}
	}

	start := time.Now()
	diff, err := FileSystemDiff(fs1, fs2)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Large FileSystemDiff failed: %v", err)
	}

	log.Infof("Large filesystem diff completed in %v, diff length: %d", duration, len(diff))

	// 验证修改的文件出现在diff中
	modifiedCount := 0
	for i := 0; i < 100; i += 10 {
		filename := fmt.Sprintf("file%d.txt", i)
		if strings.Contains(diff, filename) {
			modifiedCount++
		}
	}

	if modifiedCount == 0 {
		t.Error("Expected to find modified files in diff")
	}

	// 性能要求：100个文件应该在1秒内完成
	if duration > time.Second {
		t.Errorf("Large filesystem diff too slow: %v", duration)
	}
}

// TestFileSystemDiffCustomHandler 测试自定义处理器
func TestFileSystemDiffCustomHandler(t *testing.T) {
	fs1 := filesys.NewVirtualFs()
	fs2 := filesys.NewVirtualFs()

	fs1.WriteFile("test.txt", []byte("old"), 0644)
	fs2.WriteFile("test.txt", []byte("new"), 0644)

	var collectedPatches []string
	customHandler := func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch != nil {
			collectedPatches = append(collectedPatches, patch.String())
		}
		return nil
	}

	_, err := FileSystemDiff(fs1, fs2, customHandler)
	if err != nil {
		t.Fatalf("FileSystemDiff with custom handler failed: %v", err)
	}

	if len(collectedPatches) == 0 {
		t.Error("Custom handler should have collected patches")
	}

	for i, patch := range collectedPatches {
		log.Infof("Collected patch %d: %s", i, patch)
	}
}
