package trafficfixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RepositoryRoot is the source checkout associated with the verified index.
func RepositoryRoot() (string, error) {
	c, err := load()
	if err != nil {
		return "", err
	}
	return c.root, nil
}

// Verify checks every member, including answer inventories without path aliases.
func Verify() error {
	c, err := load()
	if err != nil {
		return err
	}
	for _, b := range c.index.Batches {
		for name := range b.Members {
			if _, err := readMember(b, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Export writes a verified workspace into an existing empty, nonsymlink
// directory. It includes archived tools, inputs and answers, plus the small
// source dependencies needed to build their Go generators. Callers own its
// lifetime. Normal traffic tests continue reading ZIPs without extraction.
func Export(directory string) error {
	names, err := Names()
	if err != nil {
		return err
	}
	return exportNames(directory, names)
}

// ExportTools materializes all archived tools, inputs and answers, but only
// requested versions of the large unified test inventories. The immutable
// ZIPs/index remain present, so omitted historical inventories remain readable
// through ReadFile. The standalone inventory checker receives only its script
// and explicitly requested inventory; it has no other archived dependencies.
// This preserves the existing expanded workspace budget.
func ExportTools(directory string, requested []string) error {
	names, err := Names()
	if err != nil {
		return err
	}
	for _, name := range requested {
		if !validName(name) {
			return fmt.Errorf("trafficfixture: invalid requested export path %q", name)
		}
	}
	if standaloneInventoryRequest(requested) {
		known := map[string]bool{}
		for _, name := range names {
			known[name] = true
		}
		if known[requested[0]] && known[requested[1]] {
			return exportNames(directory, requested)
		}
	}
	selected := make([]string, 0, len(names))
	for _, name := range names {
		include := !strings.HasPrefix(filepath.Base(name), "required-tests-traffic-unified-v")
		for _, req := range requested {
			if name == req || strings.HasPrefix(name, req+"/") {
				include = true
			}
		}
		if include {
			selected = append(selected, name)
		}
	}
	return exportNames(directory, selected)
}

// Only this exact two-file invocation is dependency-free. Generators, prefix
// requests and additional supporting inputs retain the ordinary tool workspace.
func standaloneInventoryRequest(requested []string) bool {
	if len(requested) != 2 {
		return false
	}
	const checker = "scripts/protocol-tests/check_go_test_json_v2.py"
	const prefix = "scripts/protocol-tests/required-tests-traffic-unified-v"
	a, b := requested[0], requested[1]
	if b == checker {
		a, b = b, a
	}
	if a != checker || !strings.HasPrefix(b, prefix) || !strings.HasSuffix(b, ".json") {
		return false
	}
	version := strings.TrimSuffix(strings.TrimPrefix(b, prefix), ".json")
	if version == "" {
		return false
	}
	for _, digit := range version {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func exportNames(directory string, names []string) error {
	c, err := load()
	if err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("trafficfixture: export requires an empty nonsymlink directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		return fmt.Errorf("trafficfixture: export directory must be empty")
	}
	const maxWorkspace = 384 << 20
	var total int64
	manifest := map[string]Member{}
	write := func(name string, data []byte) error {
		if !validName(name) {
			return fmt.Errorf("trafficfixture: invalid export path %q", name)
		}
		total += int64(len(data))
		if total > maxWorkspace {
			return fmt.Errorf("trafficfixture: export expanded byte limit exceeded")
		}
		filename := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return err
		}
		mode := fs.FileMode(0600)
		if bytes.HasPrefix(data, []byte("#!")) {
			mode = 0700
		}
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		manifest[name] = Member{SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: int64(len(data))}
		return nil
	}
	for _, name := range names {
		loc, ok := c.aliases[name]
		if !ok {
			return fmt.Errorf("trafficfixture: unknown export alias %q", name)
		}
		data, err := readMember(loc.batch, loc.member)
		if err != nil {
			return err
		}
		if err := write(name, data); err != nil {
			return err
		}
	}
	if err := write(batchDir+"/index.json", c.indexRaw); err != nil {
		return err
	}
	for _, b := range c.index.Batches {
		data, err := readBoundedFile(filepath.Join(c.root, batchDir, b.File), maxBatch)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != b.SHA256 {
			return fmt.Errorf("trafficfixture: batch changed during export: %s", b.ID)
		}
		if err := write(batchDir+"/"+b.File, data); err != nil {
			return err
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := readBoundedFile(filepath.Join(c.root, name), maxMember)
		if err != nil {
			return err
		}
		if err := write(name, data); err != nil {
			return err
		}
	}
	for _, dir := range []string{"internal/trafficfixture", "common/bin-parser/internal/corpusutil", "common/bin-parser/testdata/winlab5013"} {
		entries, err := os.ReadDir(filepath.Join(c.root, dir))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			name := dir + "/" + entry.Name()
			data, err := readBoundedFile(filepath.Join(c.root, name), maxMember)
			if err != nil {
				return err
			}
			if err := write(name, data); err != nil {
				return err
			}
		}
	}
	data, err := json.MarshalIndent(struct {
		SourceRoot string            `json:"source_root"`
		Files      map[string]Member `json:"files"`
	}{c.root, manifest}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "EXPORT-MANIFEST.json"), append(data, '\n'), 0600)
}
