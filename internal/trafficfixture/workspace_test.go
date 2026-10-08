package trafficfixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceExportPreservesSealedFiles(t *testing.T) {
	directory := t.TempDir()
	if err := Export(directory); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "EXPORT-MANIFEST.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files map[string]Member `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		got, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		want := manifest.Files[name]
		if int64(len(got)) != want.Bytes || fmt.Sprintf("%x", sha256.Sum256(got)) != want.SHA256 {
			t.Fatalf("export content changed: %s", name)
		}
	}
	key := "common/bin-parser/testdata/protocol-native/dns-doh/h1.keys"
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReadFile(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, filepath.FromSlash(key))
	info, err := os.Stat(filename)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("session fixture keylog must remain private in the exported workspace")
	}
	if err := os.WriteFile(filename, []byte("modified export"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := ReadFile(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil || !bytes.Equal(want, again) {
		t.Fatal("export mutation changed sealed source bytes", err)
	}
}

func TestWorkspaceExportDoesNotOverwrite(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "user-work")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if Export(directory) == nil {
		t.Fatal("accepted occupied output directory")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "preserve" {
		t.Fatal("modified existing user work", err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if Export(link) == nil {
		t.Fatal("accepted symlink output directory")
	}
}

func TestUnindexedSupportingMaterialCannotUseLooseCopy(t *testing.T) {
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(filepath.Join(root, "common/bin-parser/testdata"), "unindexed-*.keys")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString("unverified loose keylog"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(f.Name()); err == nil {
		t.Fatal("read an unindexed loose corpus keylog")
	}
}
