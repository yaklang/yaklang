package trafficfixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWorkspaceExportPreservesSealedFiles(t *testing.T) {
	t.Run("requested-inventory", workspaceToolsRetainRequestedInventory)
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	// Preserve every historical alias assertion in bounded windows; Export's
	// single-workspace budget remains unchanged as the immutable history grows.
	window := []string{}
	var size int64
	verify := func() {
		directory := t.TempDir()
		// Every byte/hash assertion for this window finishes before the next
		// export. Retaining all verified windows until the parent test exits
		// multiplies peak disk use as immutable inventory history grows.
		defer func() {
			if err := os.RemoveAll(directory); err != nil {
				t.Errorf("release verified export window: %v", err)
			}
		}()
		if err := exportNames(directory, window); err != nil {
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
		for _, name := range window {
			got, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
			if err != nil {
				t.Fatal(err)
			}
			want := manifest.Files[name]
			if int64(len(got)) != want.Bytes || fmt.Sprintf("%x", sha256.Sum256(got)) != want.SHA256 {
				t.Fatalf("export content changed: %s", name)
			}
			source, err := ReadFile(filepath.Join(c.root, filepath.FromSlash(name)))
			if err != nil || !bytes.Equal(source, got) {
				t.Fatalf("export/source mismatch: %s %v", name, err)
			}
		}
	}
	for _, name := range names {
		loc := c.aliases[name]
		n := loc.batch.Members[loc.member].Bytes
		if size+n > 128<<20 && len(window) > 0 {
			verify()
			window = nil
			size = 0
		}
		window = append(window, name)
		size += n
	}
	if len(window) > 0 {
		verify()
	}
	directory := t.TempDir()
	if err := ExportTools(directory, nil); err != nil {
		t.Fatal(err)
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

func workspaceToolsRetainRequestedInventory(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	var requested string
	for _, name := range names {
		if strings.HasPrefix(filepath.Base(name), "required-tests-traffic-unified-v") {
			requested = name
		}
	}
	if requested == "" {
		t.Fatal("no sealed inventory")
	}
	directory := t.TempDir()
	if err := ExportTools(directory, []string{requested}); err != nil {
		t.Fatal(err)
	}
	root, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		omitted := strings.HasPrefix(filepath.Base(name), "required-tests-traffic-unified-v") && name != requested
		got, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if omitted {
			if !os.IsNotExist(err) {
				t.Fatalf("unexpected historical materialization: %s %v", name, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("missing selected alias %s: %v", name, err)
		}
		want, err := ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(want, got) {
			t.Fatalf("selected alias changed %s: %v", name, err)
		}
	}
	for _, name := range []string{"../escape", "/absolute", "bad\\path"} {
		if ExportTools(t.TempDir(), []string{name}) == nil {
			t.Fatalf("accepted invalid selection %q", name)
		}
	}
}

// The standalone inventory checker needs exactly its script and one full
// inventory. Expanding unrelated input aliases must not exhaust its budget.
func workspaceStandaloneInventory(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	const checker = "scripts/protocol-tests/check_go_test_json_v2.py"
	const prefix = "scripts/protocol-tests/required-tests-traffic-unified-v"
	var inventory string
	latest := 0
	for _, name := range names {
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".json") {
			version, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json"))
			if err == nil && version > latest {
				latest, inventory = version, name
			}
		}
	}
	if inventory == "" {
		t.Fatal("missing inventory")
	}
	for _, request := range [][]string{{checker, inventory}, {inventory, checker}} {
		if !standaloneInventoryRequest(request) {
			t.Fatalf("exact checker request rejected: %v", request)
		}
	}
	for _, request := range [][]string{nil, {checker}, {inventory}, {checker, checker}, {checker, inventory, "extra"}, {checker, "scripts/protocol-tests"}, {checker, prefix + ".json"}, {checker, prefix + "189x.json"}, {checker, "elsewhere/required-tests-traffic-unified-v189.json"}} {
		if standaloneInventoryRequest(request) {
			t.Fatalf("ordinary request narrowed: %v", request)
		}
	}
	directory := t.TempDir()
	if err := ExportTools(directory, []string{checker, inventory}); err != nil {
		t.Fatal(err)
	}
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(directory, "EXPORT-MANIFEST.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files map[string]Member `json:"files"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		_, included := manifest.Files[name]
		wantIncluded := name == checker || name == inventory
		if included != wantIncluded {
			t.Fatalf("standalone alias selection %s: %v, want %v", name, included, wantIncluded)
		}
		if !wantIncluded {
			if _, err := os.Lstat(filepath.Join(directory, filepath.FromSlash(name))); !os.IsNotExist(err) {
				t.Fatalf("unrequested alias materialized %s: %v", name, err)
			}
		}
	}
	for name, pin := range manifest.Files {
		got, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(got)) != pin.Bytes || fmt.Sprintf("%x", sha256.Sum256(got)) != pin.SHA256 {
			t.Fatalf("standalone export hash/length %s", name)
		}
	}
	for _, name := range []string{checker, inventory, batchDir + "/index.json", "go.mod", "go.sum", "internal/trafficfixture/workspace.go"} {
		got, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		var want []byte
		if name == checker || name == inventory {
			want, err = ReadFile(filepath.Join(c.root, filepath.FromSlash(name)))
		} else {
			want, err = os.ReadFile(filepath.Join(c.root, filepath.FromSlash(name)))
		}
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("standalone supporting bytes changed %s: %v", name, err)
		}
	}
	for _, batch := range physicalArchives(c) {
		pin, ok := manifest.Files[batchDir+"/"+batch.File]
		if !ok || pin.Bytes <= 0 || pin.SHA256 != batch.SHA256 {
			t.Fatalf("historical batch omitted/changed %s", batch.ID)
		}
	}
	// The reduced selection must not relax existing destination safeguards.
	if err := ExportTools(directory, []string{inventory, checker}); err == nil {
		t.Fatal("overwrote occupied standalone workspace")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if err := ExportTools(link, []string{checker, inventory}); err == nil {
		t.Fatal("accepted standalone symlink destination")
	}
}
