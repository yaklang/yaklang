package scannode

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeThroughTaskLog exercises openTaskLogWriter the way executeScript would:
// it writes a payload through the returned writer (simulating yak engine stdout
// being tee'd) and verifies the per-task file receives it.
func writeThroughTaskLog(t *testing.T, dir, jobID, subTaskID, runtimeID, payload string) string {
	t.Helper()
	t.Setenv("SCANNODE_TASK_LOG_DIR", dir)
	w, closeFn := openTaskLogWriter(jobID, subTaskID, runtimeID)
	defer closeFn()
	if w == nil {
		t.Fatalf("expected non-nil writer when SCANNODE_TASK_LOG_DIR set, got nil")
	}
	if _, err := io.WriteString(w, payload); err != nil {
		t.Fatalf("write to task log writer failed: %v", err)
	}
	name := sanitizeLogName(jobID) + "_" + sanitizeLogName(subTaskID) + "_" + sanitizeLogName(runtimeID) + ".log"
	return filepath.Join(dir, name)
}

func TestOpenTaskLogWriter_DisabledByDefault(t *testing.T) {
	t.Setenv("SCANNODE_TASK_LOG_DIR", "")
	w, closeFn := openTaskLogWriter("job-1", "sub-1", "att-1")
	defer closeFn()
	if w != nil {
		t.Fatalf("expected nil writer when SCANNODE_TASK_LOG_DIR empty, got %T", w)
	}
}

func TestOpenTaskLogWriter_CreatesPerTaskFile(t *testing.T) {
	dir := t.TempDir()
	payload := "yak engine: scanning\nrule hit: cwe-79\n"
	path := writeThroughTaskLog(t, dir, "job-42", "sub-7", "att-3", payload)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read per-task log failed: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("per-task log content mismatch\nwant: %q\nhave: %q", payload, string(got))
	}
	if !strings.HasSuffix(path, "job-42_sub-7_att-3.log") {
		t.Fatalf("unexpected per-task log filename: %s", path)
	}
}

func TestOpenTaskLogWriter_AppendsAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SCANNODE_TASK_LOG_DIR", dir)
	// 第一次写
	w1, close1 := openTaskLogWriter("job-1", "sub-1", "att-1")
	if w1 == nil {
		t.Fatal("expected non-nil writer")
	}
	if _, err := io.WriteString(w1, "first line\n"); err != nil {
		t.Fatal(err)
	}
	close1()
	// 同一任务第二次写，应追加而非覆盖
	w2, close2 := openTaskLogWriter("job-1", "sub-1", "att-1")
	if w2 == nil {
		t.Fatal("expected non-nil writer")
	}
	if _, err := io.WriteString(w2, "second line\n"); err != nil {
		t.Fatal(err)
	}
	close2()

	name := sanitizeLogName("job-1") + "_" + sanitizeLogName("sub-1") + "_" + sanitizeLogName("att-1") + ".log"
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	want := "first line\nsecond line\n"
	if string(got) != want {
		t.Fatalf("append mismatch\nwant: %q\nhave: %q", want, string(got))
	}
}

func TestOpenTaskLogWriter_DegradedOnBadDir(t *testing.T) {
	// 指向一个不存在的深层路径（父目录不可创建，比如无权限的根下），
	// 应降级为 nil writer 而非 panic。用一个必然无法创建的路径。
	t.Setenv("SCANNODE_TASK_LOG_DIR", "/nonexistent-root-dir-for-test/tasks")
	w, closeFn := openTaskLogWriter("job-1", "sub-1", "att-1")
	defer closeFn()
	if w != nil {
		t.Fatalf("expected nil writer on bad dir, got %T", w)
	}
}

func TestSanitizeLogName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "_"},
		{"job/1", "job_1"},
		{"sub\\2", "sub_2"},
		{"att\x00x", "att_x"},
		{"normal", "normal"},
		{"   ", "_"},
	}
	for _, c := range cases {
		if got := sanitizeLogName(c.in); got != c.want {
			t.Errorf("sanitizeLogName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOpenTaskLogWriter_MissingFieldsUsePlaceholder(t *testing.T) {
	dir := t.TempDir()
	path := writeThroughTaskLog(t, dir, "", "", "", "payload\n")
	// 三个空字段各自 sanitize 为 "_"，用 "_" 连接 + ".log":
	// fmt.Sprintf("%s_%s_%s.log", "_","_","_") = "_"+"_"+"_"+"_"+"_" = "_____" + ".log"
	want := filepath.Join(dir, "_____.log")
	if path != want {
		t.Errorf("placeholder filename mismatch\nwant: %s\nhave: %s", want, path)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("placeholder file not readable: %v", err)
	}
}