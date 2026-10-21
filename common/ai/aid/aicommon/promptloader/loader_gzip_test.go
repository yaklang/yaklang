//go:build gzip_embed

package promptloader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCompressedPromptArchiveHasNoDeletedSources(t *testing.T) {
	if _, err := ReadFile("mainloop/workspace.txt"); err != nil {
		t.Fatal(err)
	}
	err := fs.WalkDir(archive, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if _, err := os.Stat(filepath.FromSlash(name)); err != nil {
			return fmt.Errorf("archived prompt has no source %q: %w", name, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
