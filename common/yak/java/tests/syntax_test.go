package tests

import (
	"embed"
	"path/filepath"
	"strings"
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

//go:embed code/***
var codeFs embed.FS

func TestJavaBasicParse_MockFile(t *testing.T) {
	entry, err := codeFs.ReadDir("code")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		path := filepath.Join("code", f.Name())
		if !strings.HasSuffix(path, ".java") {
			continue
		}
		raw, err := codeFs.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", path)
		}
		t.Run(path, func(t *testing.T) {
			test.MockSSA(t, string(raw))
		})
	}
}
