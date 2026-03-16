package compiler

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyFilePreserveMode_SameFileViaSymlink_NoTruncate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior/permissions differ on windows")
	}

	tmp := t.TempDir()
	realPath := filepath.Join(tmp, "a.bin")
	linkPath := filepath.Join(tmp, "b.bin")

	const payload = "hello yak"
	if err := os.WriteFile(realPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write real file: %v", err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	// Should detect same inode and do nothing (must not truncate).
	if err := CopyFilePreserveMode(realPath, linkPath); err != nil {
		t.Fatalf("copy real->link: %v", err)
	}
	if b, err := os.ReadFile(realPath); err != nil || string(b) != payload {
		t.Fatalf("real file changed after copy to symlink: err=%v payload=%q", err, b)
	}

	// Reverse direction should also do nothing.
	if err := CopyFilePreserveMode(linkPath, realPath); err != nil {
		t.Fatalf("copy link->real: %v", err)
	}
	if b, err := os.ReadFile(realPath); err != nil || string(b) != payload {
		t.Fatalf("real file changed after copy from symlink: err=%v payload=%q", err, b)
	}
}

func TestCopyFilePreserveMode_SameFileAfterClean_NoWork(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "x.bin")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := CopyFilePreserveMode(filepath.Join(tmp, ".", "x.bin"), p); err != nil {
		t.Fatalf("copy cleaned path: %v", err)
	}
}
