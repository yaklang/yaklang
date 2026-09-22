package testcheck

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FixtureFS opens test-only embedded ZIPs under their original logical paths.
// Entries decompress on first access, in memory only. Readers and returned byte
// slices are independent, so a mutation test cannot corrupt the shared corpus.
// This package is imported only by tests; it embeds no assets itself.
type FixtureFS struct {
	entries map[string]*fixtureEntry
	dirs    map[string][]fs.DirEntry
}
type fixtureEntry struct {
	file *zip.File
	once sync.Once
	data []byte
	err  error
}

func LoadFixtures(archives fs.FS) (*FixtureFS, error) {
	out := &FixtureFS{entries: map[string]*fixtureEntry{}, dirs: map[string][]fs.DirEntry{".": nil}}
	err := fs.WalkDir(archives, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(name, ".zip") {
			return fmt.Errorf("non-ZIP fixture archive: %s", name)
		}
		raw, err := fs.ReadFile(archives, name)
		if err != nil {
			return err
		}
		reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, file := range reader.File {
			if !fs.ValidPath(file.Name) || strings.Contains(file.Name, "\\") || !file.Mode().IsRegular() {
				return fmt.Errorf("invalid fixture entry %s:%s", name, file.Name)
			}
			logical := path.Join(path.Dir(name), file.Name)
			if _, exists := out.entries[logical]; exists {
				return fmt.Errorf("duplicate fixture: %s", logical)
			}
			out.entries[logical] = &fixtureEntry{file: file}
			dir := path.Dir(logical)
			out.dirs[dir] = append(out.dirs[dir], fs.FileInfoToDirEntry(file.FileInfo()))
			for dir != "." {
				parent := path.Dir(dir)
				if _, ok := out.dirs[parent]; !ok {
					out.dirs[parent] = nil
				}
				dir = parent
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for dir := range out.dirs {
		if dir != "." {
			parent := path.Dir(dir)
			out.dirs[parent] = append(out.dirs[parent], fs.FileInfoToDirEntry(fixtureDirInfo{path.Base(dir)}))
		}
	}
	for dir := range out.dirs {
		sort.Slice(out.dirs[dir], func(i, j int) bool { return out.dirs[dir][i].Name() < out.dirs[dir][j].Name() })
	}
	return out, nil
}
func MustLoadFixtures(archives fs.FS) *FixtureFS {
	f, err := LoadFixtures(archives)
	if err != nil {
		panic(err)
	}
	return f
}
func fixturePath(name string) string { return filepath.ToSlash(name) }
func (f *FixtureFS) data(name string) ([]byte, error) {
	name = fixturePath(name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	entry, ok := f.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	entry.once.Do(func() {
		r, err := entry.file.Open()
		if err != nil {
			entry.err = err
			return
		}
		defer r.Close()
		entry.data, entry.err = io.ReadAll(r) // EOF verifies the archive CRC.
	})
	return entry.data, entry.err
}
func (f *FixtureFS) ReadFile(name string) ([]byte, error) {
	data, err := f.data(name)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(data), nil
}
func (f *FixtureFS) OpenReader(name string) (*FixtureFile, error) {
	data, err := f.data(name)
	if err != nil {
		return nil, err
	}
	return &FixtureFile{reader: bytes.NewReader(data), info: f.entries[fixturePath(name)].file.FileInfo()}, nil
}
func (f *FixtureFS) Open(name string) (fs.File, error) {
	name = fixturePath(name)
	if _, ok := f.entries[name]; ok {
		return f.OpenReader(name)
	}
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if _, ok := f.dirs[name]; !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &fixtureDirectory{owner: f, name: name}, nil
}
func (f *FixtureFS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	dir, ok := file.(fs.ReadDirFile)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	return dir.ReadDir(-1)
}
func (f *FixtureFS) Stat(name string) (fs.FileInfo, error) {
	name = fixturePath(name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if e, ok := f.entries[name]; ok {
		return e.file.FileInfo(), nil
	}
	if _, ok := f.dirs[name]; ok {
		return fixtureDirInfo{path.Base(name)}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (f *FixtureFS) DirFS(name string) fs.FS {
	sub, err := fs.Sub(f, fixturePath(name))
	if err != nil {
		panic(err)
	}
	return sub
}
func (f *FixtureFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	return fs.WalkDir(f, fixturePath(root), fn)
}
func (f *FixtureFS) Walk(root string, fn filepath.WalkFunc) error {
	return f.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(name, nil, err)
		}
		info, err := d.Info()
		return fn(name, info, err)
	})
}

// FixtureFile retains the ReaderAt/Seek/Stat contract used by binary parsers.
type FixtureFile struct {
	reader *bytes.Reader
	info   fs.FileInfo
	closed bool
}

func (f *FixtureFile) Read(p []byte) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.Read(p)
}
func (f *FixtureFile) ReadAt(p []byte, off int64) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.ReadAt(p, off)
}
func (f *FixtureFile) Seek(off int64, whence int) (int64, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.Seek(off, whence)
}
func (f *FixtureFile) Stat() (fs.FileInfo, error) {
	if f.closed {
		return nil, fs.ErrClosed
	}
	return f.info, nil
}
func (f *FixtureFile) Close() error {
	if f.closed {
		return fs.ErrClosed
	}
	f.closed = true
	return nil
}

// Directory metadata is indexed without decompressing fixture bodies.
type fixtureDirInfo struct{ name string }

func (d fixtureDirInfo) Name() string       { return d.name }
func (d fixtureDirInfo) Size() int64        { return 0 }
func (d fixtureDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0555 }
func (d fixtureDirInfo) ModTime() time.Time { return time.Time{} }
func (d fixtureDirInfo) IsDir() bool        { return true }
func (d fixtureDirInfo) Sys() any           { return nil }

type fixtureDirectory struct {
	owner  *FixtureFS
	name   string
	offset int
	closed bool
}

func (d *fixtureDirectory) Stat() (fs.FileInfo, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}
	return d.owner.Stat(d.name)
}
func (d *fixtureDirectory) Read([]byte) (int, error) {
	if d.closed {
		return 0, fs.ErrClosed
	}
	return 0, fs.ErrInvalid
}
func (d *fixtureDirectory) Close() error {
	if d.closed {
		return fs.ErrClosed
	}
	d.closed = true
	return nil
}
func (d *fixtureDirectory) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}
	entries := d.owner.dirs[d.name]
	if n > 0 && d.offset == len(entries) {
		return nil, io.EOF
	}
	end := len(entries)
	if n > 0 && n < end-d.offset {
		end = d.offset + n
	}
	out := append([]fs.DirEntry{}, entries[d.offset:end]...)
	d.offset = end
	return out, nil
}
