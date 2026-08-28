package memedit

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// TestMemEditorConcurrentLineMappingsNoPanic 模拟 ssa 扫描期行为：同一个
// MemEditor 被多个 goroutine 并发首次访问（ssadb.irSourceCache 按 hash 共享
// editor）。旧实现直接写 lineStartOffsetMap/lineLensMap 两个字段，读者可能
// 观察到两个字段来自不同 goroutine 的构建结果（长度不一致），
// GetEndOffsetByLine 会越界 panic。修复后行映射以不可变快照原子发布，
// 并发访问必须稳定返回一致结果。
func TestMemEditorConcurrentLineMappingsNoPanic(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	editor := NewMemEditor(sb.String())
	const lineCount = 2001 // 2000 行内容 + 结尾换行产生的空行

	const workers = 32
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			gotLineCount := editor.GetLineCount()
			if gotLineCount != lineCount {
				errCh <- fmt.Errorf("GetLineCount() = %d, want %d", gotLineCount, lineCount)
				return
			}
			// runeOffsetMap 同样是懒构建缓存，必须并发安全。
			if m := editor.GetRuneOffsetMap(); m == nil || m.RuneCount() != utf8.RuneCountInString(editor.GetSourceCode()) {
				errCh <- fmt.Errorf("GetRuneOffsetMap() mismatch")
				return
			}
			for i := 1; i <= gotLineCount; i++ {
				line, err := editor.GetLine(i)
				if err != nil {
					errCh <- fmt.Errorf("GetLine(%d): %v", i, err)
					return
				}
				want := fmt.Sprintf("line %d", i-1)
				if i == gotLineCount {
					want = ""
				}
				if line != want {
					errCh <- fmt.Errorf("GetLine(%d) = %q, want %q", i, line, want)
					return
				}
				if _, err := editor.GetEndOffsetByLine(i); err != nil {
					errCh <- fmt.Errorf("GetEndOffsetByLine(%d): %v", i, err)
					return
				}
				if _, err := editor.GetStartOffsetByLine(i); err != nil {
					errCh <- fmt.Errorf("GetStartOffsetByLine(%d): %v", i, err)
					return
				}
			}
			// 最后一行的下一行必须返回错误而不是 panic。
			if _, err := editor.GetEndOffsetByLine(gotLineCount + 1); err == nil {
				errCh <- fmt.Errorf("GetEndOffsetByLine(%d) should fail", gotLineCount+1)
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

// TestMemEditorOutOfRangeLineNoPanic 覆盖 hadoop run19 的越界场景：文件行数
// 恰为 1408 时，查询 1409 行及以后必须返回错误而不是越界 panic。
func TestMemEditorOutOfRangeLineNoPanic(t *testing.T) {
	editor := NewMemEditor(strings.Repeat("line\n", 1408))
	if got := editor.GetLineCount(); got != 1409 {
		t.Fatalf("GetLineCount() = %d, want 1409", got)
	}
	if line, err := editor.GetLine(1409); err != nil || line != "" {
		t.Fatalf("GetLine(1409) = (%q, %v), want (\"\", nil)", line, err)
	}
	for _, bad := range []int{1410, 5000} {
		if _, err := editor.GetLine(bad); err == nil {
			t.Fatalf("GetLine(%d) should fail for out of range line", bad)
		}
		if _, err := editor.GetEndOffsetByLine(bad); err == nil {
			t.Fatalf("GetEndOffsetByLine(%d) should fail for out of range line", bad)
		}
	}
}
