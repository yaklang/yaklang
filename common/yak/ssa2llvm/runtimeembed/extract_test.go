package runtimeembed

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
