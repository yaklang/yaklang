package runtimeembed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

type fakeReadFileFS struct {
	files map[string][]byte
}

func (f *fakeReadFileFS) ReadFile(name string) ([]byte, error) {
	if f == nil || f.files == nil {
		return nil, errors.New("no files")
	}
	data, ok := f.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

func TestExtractLibyakToDirFromFS(t *testing.T) {
	dir := t.TempDir()

	want := []byte("fake-libyak")
	fs := &fakeReadFileFS{
		files: map[string][]byte{
			embeddedArchivePath: want,
		},
	}

	out, err := ExtractLibyakToDirFromFS(fs, dir)
	if err != nil {
		t.Fatalf("ExtractLibyakToDirFromFS failed: %v", err)
	}
	if filepath.Base(out) != archiveBaseName {
		t.Fatalf("unexpected output base name: %s", out)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output failed: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("unexpected content: %q", string(got))
	}
}

func TestExtractRuntimeSourceToDirFromFS(t *testing.T) {
	dir := t.TempDir()

	fs := fstest.MapFS{
		embeddedSrcPrefix + "/go.mod":                                              {Data: []byte("module github.com/yaklang/yaklang\n")},
		embeddedSrcPrefix + "/common/yak/ssa2llvm/runtime/runtime_go/yak_lib.go":   {Data: []byte("package main\n")},
		embeddedSrcPrefix + "/common/yak/ssa2llvm/runtime/runtime_go/libs/libgc.a": {Data: []byte("fake-gc")},
	}

	out, err := ExtractRuntimeSourceToDirFromFS(fs, dir)
	if err != nil {
		t.Fatalf("ExtractRuntimeSourceToDirFromFS failed: %v", err)
	}
	if out != dir {
		t.Fatalf("unexpected output dir: %s", out)
	}

	got, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod failed: %v", err)
	}
	if !strings.Contains(string(got), "module github.com/yaklang/yaklang") {
		t.Fatalf("unexpected go.mod content: %q", string(got))
	}

	got, err = os.ReadFile(filepath.Join(dir, "common/yak/ssa2llvm/runtime/runtime_go/libs/libgc.a"))
	if err != nil {
		t.Fatalf("read libgc.a failed: %v", err)
	}
	if string(got) != "fake-gc" {
		t.Fatalf("unexpected libgc content: %q", string(got))
	}
}
