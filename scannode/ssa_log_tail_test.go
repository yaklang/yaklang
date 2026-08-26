package scannode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestLog(t *testing.T, dir, jobID, subtaskID, attemptID string, lines []string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	path := filepath.Join(dir, sanitizeLogName(jobID)+"_"+sanitizeLogName(subtaskID)+"_"+sanitizeLogName(attemptID)+".log")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	return path
}

func TestTailLogFileReturnsTailChunkAlignedToLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestLog(t, dir, "job-a", "sub-a", "attempt-b", []string{
		"line-01",
		"line-02",
		"line-03",
		"line-04",
		"line-05",
		"line-06",
	})

	// Total content is 6 lines of 7 bytes + newline = 48 bytes. A 24-byte
	// chunk from the end starts exactly at line-04; line alignment therefore
	// drops line-04's partial boundary and returns the last two full lines.
	content, total, start, hasMore, err := tailLogFile(path, 0, 24)
	if err != nil {
		t.Fatalf("tail log: %v", err)
	}
	if total != 48 {
		t.Fatalf("unexpected total: %d", total)
	}
	if content != "line-05\nline-06\n" {
		t.Fatalf("unexpected tail content: %q", content)
	}
	if !hasMore {
		t.Fatal("expected has_more=true (older lines exist)")
	}
	// start must point at line-05: 4 lines * 8 bytes = 32.
	if start != 32 {
		t.Fatalf("unexpected chunk start: %d", start)
	}
}

func TestTailTaskFileWalksBackwards(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestLog(t, dir, "job-a", "sub-a", "att-b", []string{
		"line-01", "line-02", "line-03", "line-04", "line-05", "line-06",
	})

	// First chunk: last two lines (24-byte window lands on line-05 after
	// alignment, skipping 16 bytes from the end).
	first, _, _, hasMore, err := tailLogFile(path, 0, 24)
	if err != nil {
		t.Fatalf("tail log: %v", err)
	}
	if first != "line-05\nline-06\n" {
		t.Fatalf("first chunk: %q", first)
	}
	if !hasMore {
		t.Fatal("expected more content before the first chunk")
	}

	// Second chunk: skip the first chunk's aligned size (16 bytes) from the
	// end. The window [16,32) holds "line-03\nline-04\n"; line alignment
	// drops the partial line-03 and returns line-04.
	second, _, _, hasMore, err := tailLogFile(path, 16, 16)
	if err != nil {
		t.Fatalf("tail log: %v", err)
	}
	if second != "line-04\n" {
		t.Fatalf("second chunk: %q", second)
	}
	if !hasMore {
		t.Fatal("expected more content before the second chunk")
	}

	// Third chunk reaches the beginning of the file.
	third, _, _, hasMore, err := tailLogFile(path, 32, 24)
	if err != nil {
		t.Fatalf("tail log: %v", err)
	}
	if third != "line-01\nline-02\n" {
		t.Fatalf("third chunk: %q", third)
	}
	if hasMore {
		t.Fatal("expected no more content before the third chunk")
	}
}

func TestTailLogFileOffsetBeyondEOF(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestLog(t, dir, "job", "sub", "att", []string{"only-line"})
	content, total, start, hasMore, err := tailLogFile(path, 4096, 64)
	if err != nil {
		t.Fatalf("tail log: %v", err)
	}
	if content != "" || total != 10 || start != 10 || hasMore {
		t.Fatalf("unexpected result: content=%q total=%d start=%d hasMore=%v", content, total, start, hasMore)
	}
}

func TestResolveTaskLogPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	jobID := "job-123"
	attemptID := "attempt-456"
	writeTestLog(t, dir, jobID, "sub-a", attemptID, []string{"x"})
	writeTestLog(t, dir, jobID, "sub-b", "attempt-other", []string{"x"})

	bridge := &legionJobBridge{}
	got := bridge.resolveTaskLogPathWithDirs([]string{dir}, jobID, attemptID)
	if !strings.HasSuffix(got, jobID+"_sub-a_"+attemptID+".log") {
		t.Fatalf("unexpected resolved path: %q", got)
	}
	if got := bridge.resolveTaskLogPathWithDirs([]string{dir}, jobID, "missing-attempt"); got != "" {
		t.Fatalf("expected no match for missing attempt: %q", got)
	}
}
