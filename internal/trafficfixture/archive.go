// Package trafficfixture loads immutable, hash-pinned traffic test batches.
// It is test infrastructure; production packet readers do not depend on it.
package trafficfixture

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const batchDir = "common/bin-parser/testdata/corpus-batches"
const maxMember = 25 << 20
const maxBatch = 128 << 20

type Member struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type Batch struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind,omitempty"`
	SourceSHA string            `json:"source_sha,omitempty"`
	File      string            `json:"file"`
	SHA256    string            `json:"sha256"`
	Password  string            `json:"password"`
	Members   map[string]Member `json:"members"`
	Aliases   map[string]string `json:"aliases"`
	files     map[string]*zip.File
}
type Index struct {
	SchemaVersion       int      `json:"schema_version"`
	SourceSHA           string   `json:"source_sha"`
	SourceArchiveSHA256 string   `json:"source_archive_sha256"`
	Batches             []*Batch `json:"batches"`
}
type location struct {
	batch  *Batch
	member string
}
type corpus struct {
	root     string
	index    Index
	indexRaw []byte
	aliases  map[string]location
}

var loaded struct {
	sync.Once
	corpus *corpus
	err    error
}

func validName(name string) bool {
	return fs.ValidPath(name) && name != "." && !strings.ContainsAny(name, "\\:")
}

// openBatch checks the compressed archive before trusting its directory. No
// member is extracted to the repository, including unknown or unsafe names.
func openBatch(raw []byte, b *Batch) error {
	if len(raw) > maxBatch || fmt.Sprintf("%x", sha256.Sum256(raw)) != b.SHA256 {
		return fmt.Errorf("batch %s: archive size or SHA256 mismatch", b.ID)
	}
	r, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	if len(r.File) != len(b.Members) || len(r.File) > 10000 {
		return fmt.Errorf("batch %s: member inventory mismatch", b.ID)
	}
	b.files = make(map[string]*zip.File, len(r.File))
	var total uint64
	for _, f := range r.File {
		m, ok := b.Members[f.Name]
		hash, hashErr := hex.DecodeString(m.SHA256)
		if !validName(f.Name) || !ok || b.files[f.Name] != nil || !f.Mode().IsRegular() || f.UncompressedSize64 > maxMember || m.Bytes < 0 || uint64(m.Bytes) != f.UncompressedSize64 || hashErr != nil || len(hash) != sha256.Size {
			return fmt.Errorf("batch %s: invalid member %q", b.ID, f.Name)
		}
		if f.Flags&1 == 0 || f.Flags&0x40 != 0 || (f.Method != zip.Store && f.Method != zip.Deflate) {
			return fmt.Errorf("batch %s: unsupported encryption or compression: %s", b.ID, f.Name)
		}
		total += f.UncompressedSize64
		if total > maxBatch {
			return fmt.Errorf("batch %s: expanded size limit exceeded", b.ID)
		}
		b.files[f.Name] = f
	}
	for alias, member := range b.Aliases {
		if !validName(alias) || b.files[member] == nil {
			return fmt.Errorf("batch %s: invalid alias %q", b.ID, alias)
		}
	}
	return nil
}

// ZipCrypto is retained solely for the existing fixture format. The published
// fixture password is a packaging convention, not a confidentiality boundary.
type decryptReader struct {
	r    io.Reader
	keys [3]uint32
}

func (r *decryptReader) update(c byte) {
	r.keys[0] = crc32.IEEETable[(r.keys[0]^uint32(c))&255] ^ (r.keys[0] >> 8)
	r.keys[1] = (r.keys[1]+(r.keys[0]&255))*134775813 + 1
	r.keys[2] = crc32.IEEETable[(r.keys[2]^(r.keys[1]>>24))&255] ^ (r.keys[2] >> 8)
}
func (r *decryptReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	for i := 0; i < n; i++ {
		t := uint16(r.keys[2]) | 2
		p[i] ^= byte((t * (t ^ 1)) >> 8)
		r.update(p[i])
	}
	return n, err
}
func readMember(b *Batch, name string) ([]byte, error) {
	f := b.files[name]
	if f == nil {
		return nil, fmt.Errorf("batch %s: missing member %s", b.ID, name)
	}
	raw, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	d := &decryptReader{r: raw, keys: [3]uint32{0x12345678, 0x23456789, 0x34567890}}
	for _, c := range []byte(b.Password) {
		d.update(c)
	}
	var header [12]byte
	if _, err = io.ReadFull(d, header[:]); err != nil {
		return nil, err
	}
	check := byte(f.CRC32 >> 24)
	if f.Flags&8 != 0 {
		check = byte(f.ModifiedTime >> 8)
	}
	if header[11] != check {
		return nil, fmt.Errorf("batch %s: invalid password", b.ID)
	}
	var reader io.Reader = d
	if f.Method == zip.Deflate {
		inflater := flate.NewReader(d)
		defer inflater.Close()
		reader = inflater
	}
	m := b.Members[name]
	data, err := io.ReadAll(io.LimitReader(reader, m.Bytes+1))
	if err != nil {
		return nil, fmt.Errorf("batch %s member %s: %w", b.ID, name, err)
	}
	if int64(len(data)) != m.Bytes || crc32.ChecksumIEEE(data) != f.CRC32 || fmt.Sprintf("%x", sha256.Sum256(data)) != m.SHA256 {
		return nil, fmt.Errorf("batch %s: member integrity mismatch: %s", b.ID, name)
	}
	return data, nil
}

func load() (*corpus, error) {
	loaded.Do(func() {
		root, err := os.Getwd()
		if err != nil {
			loaded.err = err
			return
		}
		for {
			mod, e := os.ReadFile(filepath.Join(root, "go.mod"))
			if e == nil && bytes.Contains(mod, []byte("module github.com/yaklang/yaklang")) {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				loaded.err = fmt.Errorf("trafficfixture: repository root not found")
				return
			}
			root = parent
		}
		c := &corpus{root: root, aliases: map[string]location{}}
		data, err := readBoundedFile(filepath.Join(root, batchDir, "index.json"), maxMember)
		if err == nil {
			c.indexRaw = data
			err = json.Unmarshal(data, &c.index)
		}
		if err != nil {
			loaded.err = err
			return
		}
		if c.index.SchemaVersion != 1 || len(c.index.Batches) == 0 {
			loaded.err = fmt.Errorf("trafficfixture: unsupported index")
			return
		}
		ids := map[string]bool{}
		for _, b := range c.index.Batches {
			if b == nil || !validName(b.File) || strings.Contains(b.File, "/") || b.ID == "" || ids[b.ID] {
				loaded.err = fmt.Errorf("trafficfixture: invalid batch identity")
				return
			}
			ids[b.ID] = true
			raw, err := readBoundedFile(filepath.Join(root, batchDir, b.File), maxBatch)
			if err == nil {
				err = openBatch(raw, b)
			}
			if err != nil {
				loaded.err = err
				return
			}
			for alias, member := range b.Aliases {
				if _, ok := c.aliases[alias]; ok {
					loaded.err = fmt.Errorf("trafficfixture: duplicate alias %s", alias)
					return
				}
				c.aliases[alias] = location{b, member}
			}
		}
		loaded.corpus = c
	})
	return loaded.corpus, loaded.err
}

func readBoundedFile(name string, limit int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("trafficfixture: file size or type limit: %s", name)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("trafficfixture: file grew beyond limit: %s", name)
	}
	return data, nil
}

func resolve(c *corpus, name string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(c.root, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// ReadFile routes indexed inputs exclusively through their immutable batch.
// Non-corpus inputs (for example per-test temporary files) use the filesystem.
// A missing repository capture is an error, even if a loose copy exists.
func ReadFile(name string) ([]byte, error) {
	c, err := load()
	if err != nil {
		return nil, err
	}
	rel, err := resolve(c, name)
	if err != nil {
		return nil, err
	}
	if loc, ok := c.aliases[rel]; ok {
		return readMember(loc.batch, loc.member)
	}
	ext := strings.ToLower(path.Ext(rel))
	if validName(rel) && (ext == ".pcap" || ext == ".pcapng" || ext == ".cap") {
		return nil, fmt.Errorf("trafficfixture: unindexed repository capture %s", rel)
	}
	if validName(rel) && ext != ".go" && !strings.HasPrefix(rel, batchDir+"/") {
		for _, prefix := range []string{"common/bin-parser/testdata/", "common/pcapx/pcaputil/testdata/", "scripts/protocol-tests/"} {
			if strings.HasPrefix(rel, prefix) {
				return nil, fmt.Errorf("trafficfixture: unindexed repository material %s", rel)
			}
		}
	}
	return os.ReadFile(name)
}

type Reader struct{ *bytes.Reader }

func (r *Reader) Close() error { return nil }
func Open(name string) (*Reader, error) {
	data, err := ReadFile(name)
	if err != nil {
		return nil, err
	}
	return &Reader{bytes.NewReader(data)}, nil
}

// Materialize supplies a verified private copy for APIs requiring a filename.
// The caller owns the directory and its lifetime (normally testing.T.TempDir).
func Materialize(name, directory string) (string, error) {
	data, err := ReadFile(name)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(directory, "fixture-*"+filepath.Ext(name))
	if err != nil {
		return "", err
	}
	filename := f.Name()
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(filename)
		if writeErr != nil {
			return "", writeErr
		}
		return "", closeErr
	}
	return filename, nil
}

// Names returns repository-relative aliases, in a stable order. It does not
// extract any files and includes aliases sharing the same content hash.
func Names() ([]string, error) {
	c, err := load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(c.aliases))
	for name := range c.aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ReadBatch reads a named member without a historical filesystem alias.
func ReadBatch(id, member string) ([]byte, error) {
	c, err := load()
	if err != nil {
		return nil, err
	}
	for _, b := range c.index.Batches {
		if b.ID == id {
			return readMember(b, member)
		}
	}
	return nil, fmt.Errorf("trafficfixture: unknown batch %s", id)
}

type entry struct {
	name      string
	directory bool
	size      int64
}

func (e entry) Name() string { return e.name }
func (e entry) IsDir() bool  { return e.directory }
func (e entry) Type() fs.FileMode {
	if e.directory {
		return fs.ModeDir
	}
	return 0
}
func (e entry) Info() (fs.FileInfo, error) { return e, nil }
func (e entry) Size() int64                { return e.size }
func (e entry) Mode() fs.FileMode          { return e.Type() | 0444 }
func (e entry) ModTime() time.Time         { return time.Time{} }
func (e entry) Sys() any                   { return nil }

// ReadDir merges ordinary source files with the virtual archive inventory.
// Indexed files take precedence. Temporary directories retain OS semantics.
func ReadDir(name string) ([]fs.DirEntry, error) {
	c, err := load()
	if err != nil {
		return nil, err
	}
	rel, err := resolve(c, name)
	if err != nil {
		return nil, err
	}
	items, osErr := os.ReadDir(name)
	if osErr != nil && !os.IsNotExist(osErr) {
		return nil, osErr
	}
	entries := map[string]fs.DirEntry{}
	for _, e := range items {
		entries[e.Name()] = e
	}
	for alias, loc := range c.aliases {
		if !strings.HasPrefix(alias, rel+"/") {
			continue
		}
		rest := strings.TrimPrefix(alias, rel+"/")
		part, _, dir := strings.Cut(rest, "/")
		size := int64(0)
		if !dir {
			size = loc.batch.Members[loc.member].Bytes
		}
		entries[part] = entry{part, dir, size}
	}
	if len(entries) == 0 && osErr != nil {
		return nil, osErr
	}
	result := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name() < result[j].Name() })
	return result, nil
}

func WalkDir(root string, fn fs.WalkDirFunc) error {
	var visit func(string, fs.DirEntry) error
	visit = func(name string, d fs.DirEntry) error {
		if err := fn(name, d, nil); err != nil {
			if err == fs.SkipDir && d.IsDir() {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		children, err := ReadDir(name)
		if err != nil {
			return fn(name, d, err)
		}
		for _, child := range children {
			if err := visit(filepath.Join(name, child.Name()), child); err != nil {
				return err
			}
		}
		return nil
	}
	d := entry{filepath.Base(root), true, 0}
	if _, err := ReadDir(root); err != nil {
		return fn(root, nil, err)
	}
	err := visit(root, d)
	if err == fs.SkipAll {
		return nil
	}
	return err
}

func Glob(pattern string) ([]string, error) {
	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	c, err := load()
	if err != nil {
		return nil, err
	}
	rel, err := resolve(c, pattern)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, m := range matches {
		set[m] = true
	}
	for alias := range c.aliases {
		ok, err := filepath.Match(filepath.FromSlash(rel), filepath.FromSlash(alias))
		if err != nil {
			return nil, err
		}
		if ok {
			abs := filepath.Join(c.root, filepath.FromSlash(alias))
			if filepath.IsAbs(pattern) {
				set[abs] = true
			} else {
				cwd, err := os.Getwd()
				if err != nil {
					return nil, err
				}
				name, err := filepath.Rel(cwd, abs)
				if err != nil {
					return nil, err
				}
				set[name] = true
			}
		}
	}
	matches = matches[:0]
	for m := range set {
		matches = append(matches, m)
	}
	sort.Strings(matches)
	return matches, nil
}
