package java_decompiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestFilePath(t *testing.T) {
	jarPath := os.Getenv("TEST_JAR_PATH")
	if jarPath == "" {
		t.Skip("TEST_JAR_PATH environment variable not set, skipping test")
	}
	zipfs, err := filesys.NewZipFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("Failed to create zipfs: %v", err)
	}
	zipfs.ReadFile(filepath.Join("META-INF", "MANIFEST.MF"))
}
