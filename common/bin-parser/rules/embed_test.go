package rules

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedRulesMatchSources(t *testing.T) {
	count := 0
	err := filepath.WalkDir(".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(name, ".yaml") {
			return nil
		}
		want, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
		got, err := RuleFS.ReadFile(filepath.ToSlash(name))
		if err != nil {
			return err
		}
		if !bytes.Equal(want, got) {
			t.Errorf("stale embedded rule %s: run go generate ./common/bin-parser/rules", name)
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	embedded := 0
	err = fs.WalkDir(RuleFS, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if !strings.HasSuffix(name, ".yaml") {
				t.Errorf("non-rule embedded: %s", name)
			}
			if _, err := os.Stat(filepath.FromSlash(name)); err != nil {
				return err
			}
			embedded++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 || count != embedded {
		t.Fatalf("source/embedded count %d/%d", count, embedded)
	}
}
