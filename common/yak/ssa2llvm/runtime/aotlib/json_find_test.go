package aotlib

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestJsonFindPathString(t *testing.T) {
	raw := `{
  "properties": {
    "key": {"type": "string"},
    "@action": {"const": "aaa", "type": "string"}
  }
}`
	got := JsonFindPath(raw, "$..properties..const")
	if got != "aaa" {
		t.Fatalf("FindPath = %#v", got)
	}
	if JsonFindPath(`{"a":"a1","c":{"a":"a2"}}`, "$..a") != "a1" {
		t.Fatal("first recursive match regressed")
	}
}

func TestFileWalkCallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var names []string
	err := FileWalk(dir, func(info *utils.FileInfo) bool {
		names = append(names, info.Name)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "a.txt" {
		t.Fatalf("walk names = %#v", names)
	}
}
