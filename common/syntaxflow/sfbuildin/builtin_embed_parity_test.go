//go:build !irify_exclude

package sfbuildin

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

// A readable but partial/stale gzip archive must not pass by validating only
// the files it happens to contain. Compare the complete set and exact bytes.
func TestBuiltinRiskEmbeddedParity(t *testing.T) {
	InitEmbedFSWithNotify(nil)
	embedded := make(map[string][]byte)
	err := filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if !strings.HasSuffix(info.Name(), ".sf") {
			return nil
		}
		raw, err := ruleFSWithHash.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(strings.TrimPrefix(filepath.ToSlash(path), "./"), "buildin/")
		if _, exists := embedded[name]; exists {
			t.Errorf("duplicate embedded rule path: %s", name)
		}
		embedded[name] = raw
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	err = filepath.WalkDir("buildin", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".sf") {
			return err
		}
		count++
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(filepath.ToSlash(path), "buildin/")
		if value, ok := embedded[name]; !ok || !bytes.Equal(value, raw) {
			t.Errorf("missing or stale embedded rule: %s", name)
		}
		delete(embedded, name)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 || len(embedded) > 0 {
		t.Fatalf("source count=%d, unexpected embedded paths=%d", count, len(embedded))
	}
	t.Logf("all %d embedded rules match the checkout byte-for-byte", count)
}
