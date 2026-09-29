package promptloader

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestCompressedPromptResources runs in both build modes. It compares every
// embedded prompt with the editable source, so release archives cannot silently
// ship stale templates after a source edit.
func TestCompressedPromptResources(t *testing.T) {
	count := 0
	err := filepath.WalkDir("prompts", func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel("prompts", file)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		want, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		got, err := ReadFile(name)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			t.Errorf("prompt content differs from source: %s", name)
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no prompt resources found")
	}
	for _, name := range []string{"", "../mainloop/workspace.txt", "mainloop/../workspace.txt", "/mainloop/workspace.txt", "mainloop\\workspace.txt"} {
		if _, err := ReadFile(name); err == nil {
			t.Errorf("accepted invalid prompt path %q", name)
		}
	}
}
