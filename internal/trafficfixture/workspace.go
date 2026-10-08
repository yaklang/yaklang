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
	names, err := Names()
	if err != nil {
		return err
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
		loc := c.aliases[name]
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
