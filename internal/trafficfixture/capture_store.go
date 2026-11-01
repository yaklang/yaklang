package trafficfixture

// The outer store preserves historical batch ZIPs byte for byte. Their own
// passwords, indexes, aliases, provenance and expected answers remain unchanged.
// Only the physical checkout layout changes; production readers do not use it.
import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const captureStoreManifest = "internal/trafficfixture/testdata/captures.manifest.json"
const captureStoreArchive = "internal/trafficfixture/testdata/captures.zip"
const maxCaptureStoreBytes = 64 << 20

type captureStoreIndex struct {
	Version       int               `json:"version"`
	Archive       string            `json:"archive"`
	ArchiveBytes  int64             `json:"archive_bytes"`
	ArchiveSHA256 string            `json:"archive_sha256"`
	SourceHEAD    string            `json:"source_head"`
	Members       map[string]Member `json:"members"`
}

type captureStore struct {
	index captureStoreIndex
	files map[string]*zip.File
}

var captureStores sync.Map // absolute repository root -> immutable result

type captureStoreResult struct {
	once  sync.Once
	store *captureStore
	err   error
}

func parseCaptureStore(manifest, raw []byte) (*captureStore, error) {
	var index captureStoreIndex
	if err := strictPackJSON(manifest, &index); err != nil {
		return nil, err
	}
	if index.Version != 1 || index.Archive != captureStoreArchive || !digestValid(index.SourceHEAD, 20) ||
		index.ArchiveBytes != int64(len(raw)) || len(raw) > maxCaptureStoreBytes ||
		!digestValid(index.ArchiveSHA256, 32) || fmt.Sprintf("%x", sha256.Sum256(raw)) != index.ArchiveSHA256 ||
		len(index.Members) == 0 || len(index.Members) > 1024 {
		return nil, fmt.Errorf("trafficfixture: invalid central capture store manifest or hash")
	}
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	if len(z.File) != len(index.Members) {
		return nil, fmt.Errorf("trafficfixture: central capture store inventory mismatch")
	}
	s := &captureStore{index: index, files: map[string]*zip.File{}}
	var total uint64
	previous := ""
	folded := map[string]bool{}
	for _, f := range z.File {
		m, ok := index.Members[f.Name]
		fold := strings.ToLower(f.Name)
		if !validName(f.Name) || !ok || f.Name <= previous || s.files[f.Name] != nil || folded[fold] ||
			!f.Mode().IsRegular() || f.Flags&1 != 0 || f.Method != zip.Store ||
			m.Bytes < 0 || m.Bytes > maxCaptureStoreBytes || uint64(m.Bytes) != f.UncompressedSize64 || !digestValid(m.SHA256, 32) {
			return nil, fmt.Errorf("trafficfixture: invalid central capture member %q", f.Name)
		}
		total += f.UncompressedSize64
		if total > maxCaptureStoreBytes {
			return nil, fmt.Errorf("trafficfixture: central capture expansion limit")
		}
		previous, folded[fold], s.files[f.Name] = f.Name, true, f
	}
	return s, nil
}

func (s *captureStore) read(name string, limit int64) ([]byte, bool, error) {
	f := s.files[name]
	if f == nil {
		return nil, false, nil
	}
	m := s.index.Members[name]
	if m.Bytes > limit {
		return nil, true, fmt.Errorf("trafficfixture: central capture member exceeds caller limit: %s", name)
	}
	r, err := f.Open()
	if err != nil {
		return nil, true, err
	}
	b, readErr := io.ReadAll(io.LimitReader(r, m.Bytes+1))
	closeErr := r.Close()
	if readErr != nil {
		return nil, true, readErr
	}
	if closeErr != nil {
		return nil, true, closeErr
	}
	if int64(len(b)) != m.Bytes || fmt.Sprintf("%x", sha256.Sum256(b)) != m.SHA256 {
		return nil, true, fmt.Errorf("trafficfixture: central capture member integrity mismatch: %s", name)
	}
	return b, true, nil
}

func repositoryForFile(name string) (string, string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", "", err
	}
	for root := filepath.Dir(abs); ; root = filepath.Dir(root) {
		mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err == nil && bytes.Contains(mod, []byte("module github.com/yaklang/yaklang")) {
			rel, err := filepath.Rel(root, abs)
			return root, filepath.ToSlash(rel), err
		}
		if filepath.Dir(root) == root {
			return "", "", nil
		}
	}
}

func readCentralCapture(name string, limit int64) ([]byte, bool, error) {
	root, rel, err := repositoryForFile(name)
	if err != nil || root == "" {
		return nil, false, err
	}
	v, _ := captureStores.LoadOrStore(root, &captureStoreResult{})
	result := v.(*captureStoreResult)
	result.once.Do(func() {
		manifest, err := readDiskBoundedFile(filepath.Join(root, captureStoreManifest), 1<<20)
		if os.IsNotExist(err) {
			// Explicit exported tool workspaces retain the historical physical
			// archives and do not need a second copy of the outer store.
			return
		}
		if err != nil {
			result.err = err
			return
		}
		raw, err := readDiskBoundedFile(filepath.Join(root, captureStoreArchive), maxCaptureStoreBytes)
		if err != nil {
			result.err = err
			return
		}
		result.store, result.err = parseCaptureStore(manifest, raw)
	})
	if result.err != nil || result.store == nil {
		return nil, false, result.err
	}
	return result.store.read(rel, limit)
}

func verifyCentralCaptures(root string) error {
	// load() has already opened the store through the original batch loader.
	v, ok := captureStores.Load(root)
	if !ok {
		return nil
	}
	r := v.(*captureStoreResult)
	if r.err != nil {
		return r.err
	}
	if r.store == nil {
		return nil // Explicit compatibility export without an outer store.
	}
	for name := range r.store.index.Members {
		if _, _, err := r.store.read(name, maxCaptureStoreBytes); err != nil {
			return err
		}
	}
	return nil
}
