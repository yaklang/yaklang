package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/rust/rust2ssa"
)

func TestRustCorpusCoverage(t *testing.T) {
	root := os.Getenv("RUST_CORPUS")
	if root == "" {
		t.Skip("RUST_CORPUS not set")
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".rs") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Logf("RESULT\tFAIL\t%s\tread: %v", path, err)
			return nil
		}
		_, parseErr := rust2ssa.Frontend(string(raw))
		if parseErr != nil {
			message := strings.ReplaceAll(parseErr.Error(), "\n", " ")
			if len(message) > 300 {
				message = message[:300]
			}
			t.Logf("RESULT\tFAIL\t%s\t%s", path, message)
			return nil
		}
		t.Logf("RESULT\tPASS\t%s\t", path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
