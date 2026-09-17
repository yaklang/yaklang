// Package embeddedfs provides lazy, read-only compressed embedded resources.
package embeddedfs

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

// FS decompresses and indexes an immutable tar.zst archive on first access.
// Creation performs no decompression, goroutine creation or file allocation.
// ReadFile returns a copy, like embed.FS; Open exposes only read-only handles.
// After initialization, concurrent readers share immutable decoded bytes.
type FS struct {
	archive string
	limit   uint64
	once    sync.Once
	entries map[string]*entry
	err     error
}

// NewZstd constructs a lazy filesystem with a bound on decompressed archive size.
// Archive errors are returned consistently by the first and all later accesses.
func NewZstd(archive string, maxDecodedBytes uint64) *FS {
	return &FS{archive: archive, limit: maxDecodedBytes}
}

func (f *FS) load() {
	f.once.Do(func() { f.entries, f.err = unpack(f.archive, f.limit) })
}

func unpack(archive string, limit uint64) (map[string]*entry, error) {
	if limit == 0 {
		return nil, fmt.Errorf("embedded archive: decoded size limit is zero")
	}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(limit), zstd.WithDecodeAllCapLimit(true))
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	// The frame header is untrusted: validate its size before reserving output.
	var header zstd.Header
	if err := header.Decode([]byte(archive)); err != nil {
		return nil, err
	}
	if !header.HasFCS || header.FrameContentSize > limit || header.FrameContentSize > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("embedded archive: missing or excessive decoded size")
	}
	decoded, err := decoder.DecodeAll([]byte(archive), make([]byte, 0, int(header.FrameContentSize)))
	if err != nil {
		return nil, err
	}
	entries := map[string]*entry{".": {name: ".", mode: fs.ModeDir | 0555}}
	var directory func(string) (*entry, error)
	directory = func(name string) (*entry, error) {
		if existing := entries[name]; existing != nil {
			if !existing.IsDir() {
				return nil, fmt.Errorf("embedded archive: file/directory conflict at %q", name)
			}
			return existing, nil
		}
		if len(entries) >= 16384 {
			return nil, fmt.Errorf("embedded archive: too many entries")
		}
		parent, err := directory(path.Dir(name))
		if err != nil {
			return nil, err
		}
		e := &entry{name: path.Base(name), mode: fs.ModeDir | 0555}
		entries[name] = e
		parent.children = append(parent.children, e)
		return e, nil
	}
	reader := bytes.NewReader(decoded)
	tr := tar.NewReader(reader)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !fs.ValidPath(h.Name) || h.Name == "." {
			return nil, fmt.Errorf("embedded archive: invalid path %q", h.Name)
		}
		if h.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("embedded archive: unsupported entry %q", h.Name)
		}
		if len(entries) >= 16384 || entries[h.Name] != nil {
			return nil, fmt.Errorf("embedded archive: duplicate or excessive entries at %q", h.Name)
		}
		parent, err := directory(path.Dir(h.Name))
		if err != nil {
			return nil, err
		}
		start := int64(len(decoded)) - int64(reader.Len())
		if h.Size < 0 || h.Size > int64(reader.Len()) {
			return nil, io.ErrUnexpectedEOF
		}
		end := start + h.Size
		// The decoded archive remains owned by this FS. Disjoint, capacity-limited
		// file views avoid a second set of per-file content buffers. Next skips
		// the current body and validates padding without exposing mutable bytes.
		e := &entry{name: path.Base(h.Name), mode: 0444, data: decoded[start:end:end]}
		entries[h.Name] = e
		parent.children = append(parent.children, e)
	}
	for _, e := range entries {
		sort.Slice(e.children, func(i, j int) bool { return e.children[i].Name() < e.children[j].Name() })
	}
	return entries, nil
}

func (f *FS) lookup(op, name string) (*entry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	f.load()
	if f.err != nil {
		return nil, &fs.PathError{Op: op, Path: name, Err: f.err}
	}
	e := f.entries[name]
	if e == nil {
		return nil, &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
	}
	return e, nil
}

func (f *FS) Open(name string) (fs.File, error) {
	e, err := f.lookup("open", name)
	if err != nil {
		return nil, err
	}
	return &file{entry: e, reader: *bytes.NewReader(e.data)}, nil
}

func (f *FS) ReadFile(name string) ([]byte, error) {
	e, err := f.lookup("readfile", name)
	if err != nil {
		return nil, err
	}
	if e.IsDir() {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	return bytes.Clone(e.data), nil
}

func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	e, err := f.lookup("readdir", name)
	if err != nil {
		return nil, err
	}
	if !e.IsDir() {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	result := make([]fs.DirEntry, len(e.children))
	for i, c := range e.children {
		result[i] = c
	}
	return result, nil
}

func (f *FS) Stat(name string) (fs.FileInfo, error) {
	e, err := f.lookup("stat", name)
	if err != nil {
		return nil, err
	}
	return e, nil
}

type entry struct {
	name     string
	mode     fs.FileMode
	data     []byte
	children []*entry
}

func (e *entry) Name() string               { return e.name }
func (e *entry) Size() int64                { return int64(len(e.data)) }
func (e *entry) Mode() fs.FileMode          { return e.mode }
func (e *entry) ModTime() time.Time         { return time.Time{} }
func (e *entry) IsDir() bool                { return e.mode.IsDir() }
func (e *entry) Sys() any                   { return nil }
func (e *entry) Type() fs.FileMode          { return e.mode.Type() }
func (e *entry) Info() (fs.FileInfo, error) { return e, nil }

type file struct {
	*entry
	reader bytes.Reader
	cursor int
}

func (f *file) Stat() (fs.FileInfo, error) { return f.entry, nil }
func (f *file) Close() error               { return nil }
func (f *file) Read(p []byte) (int, error) {
	if f.IsDir() {
		return 0, fs.ErrInvalid
	}
	return f.reader.Read(p)
}
func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	if !f.IsDir() {
		return nil, fs.ErrInvalid
	}
	if n > 0 && f.cursor >= len(f.children) {
		return nil, io.EOF
	}
	end := len(f.children)
	if n > 0 && n < end-f.cursor {
		end = f.cursor + n
	}
	result := make([]fs.DirEntry, end-f.cursor)
	for i := range result {
		result[i] = f.children[f.cursor+i]
	}
	f.cursor = end
	return result, nil
}

var _ fs.ReadFileFS = (*FS)(nil)
var _ fs.ReadDirFS = (*FS)(nil)
var _ fs.StatFS = (*FS)(nil)
var _ fs.ReadDirFile = (*file)(nil)
