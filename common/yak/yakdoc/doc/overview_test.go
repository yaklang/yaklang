package doc

import (
	"strings"
	"testing"
)

// TestOverviewShort_BakedIntoGob 验证 doc.gob.zst 已包含库级 OverviewShort,
// 且 BuildLibrarySelectionIndex 能据此产出非空"库选择索引"。
// 关键词: OverviewShort, 库选择索引, doc.gob.zst, 生成期注入
func TestOverviewShort_BakedIntoGob(t *testing.T) {
	helper := GetDefaultDocumentHelper()
	if helper == nil {
		t.Fatal("default document helper should not be nil")
	}

	// 常用库应当带有一句话定位(由 overviews/<lib>.md 首段派生)
	wantNonEmpty := []string{"http", "poc", "file"}
	hit := 0
	for _, name := range wantNonEmpty {
		if strings.TrimSpace(GetLibOverviewShort(name)) != "" {
			hit++
		} else {
			t.Logf("lib %s has empty OverviewShort", name)
		}
	}
	if hit == 0 {
		t.Fatalf("none of common libs %v carry OverviewShort; gob may be stale", wantNonEmpty)
	}

	index := BuildLibrarySelectionIndex()
	if strings.TrimSpace(index) == "" {
		t.Fatal("library selection index should not be empty")
	}
	lines := strings.Count(index, "\n")
	if lines < 1 {
		t.Fatalf("library selection index should have at least 1 line, got %d", lines)
	}
	t.Logf("library selection index lines: %d", lines)
}
