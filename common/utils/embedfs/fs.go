// Package embedfs exposes individually gzip-compressed embedded files under their
// original names. It only decompresses files that are opened, with no global cache.
package embedfs

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Suffix is reserved for generated storage files, never a public resource path.
const Suffix = ".embed.gz"

// FS preserves the read-only io/fs interface of its backing embedded filesystem.
// Source must contain either a plain file or its generated compressed counterpart,
// never both. Compressed files must be single gzip members smaller than 4 GiB.
type FS struct{ source fs.FS }

func New(source fs.FS) FS { return FS{source: source} }

func valid(name string) bool { return fs.ValidPath(name) && !strings.HasSuffix(name, Suffix) }
func failure(op, name string, err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return &fs.PathError{Op: op, Path: name, Err: err}
}

func (f FS) Open(name string) (fs.File, error) {
	if !valid(name) {
		return nil, failure("open", name, fs.ErrInvalid)
	}
	file, err := f.source.Open(name)
	if err == nil {
		info, statErr := file.Stat()
		if statErr != nil {
			file.Close()
			return nil, failure("open", name, statErr)
		}
		if info.IsDir() {
			return &directory{File: file, fs: f, name: name}, nil
		}
		return file, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, failure("open", name, err)
	}
	raw, err := fs.ReadFile(f.source, name+Suffix)
	if err != nil {
		return nil, failure("open", name, err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, failure("open", name, err)
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return nil, failure("open", name, err)
	}
	info, err := fs.Stat(f.source, name+Suffix)
	if err != nil {
		return nil, failure("open", name, err)
	}
	return &fileReader{Reader: bytes.NewReader(data), data: data, info: fileInfo{FileInfo: info, name: path.Base(name), size: int64(len(data))}}, nil
}

func (f FS) ReadFile(name string) ([]byte, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if decoded, ok := file.(*fileReader); ok {
		return decoded.data, nil
	}
	return io.ReadAll(file)
}

func (f FS) Stat(name string) (fs.FileInfo, error) {
	if !valid(name) {
		return nil, failure("stat", name, fs.ErrInvalid)
	}
	info, err := fs.Stat(f.source, name)
	if err == nil {
		return info, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, failure("stat", name, err)
	}
	info, err = fs.Stat(f.source, name+Suffix)
	if err != nil {
		return nil, failure("stat", name, err)
	}
	raw, err := fs.ReadFile(f.source, name+Suffix)
	if err != nil {
		return nil, failure("stat", name, err)
	}
	// The generator emits a single gzip member. Its trailer records the original
	// size, so directory listings and Stat do not decompress resource contents.
	if len(raw) < 18 || raw[0] != 0x1f || raw[1] != 0x8b {
		return nil, failure("stat", name, gzip.ErrHeader)
	}
	return fileInfo{FileInfo: info, name: path.Base(name), size: int64(binary.LittleEndian.Uint32(raw[len(raw)-4:]))}, nil
}

func (f FS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !valid(name) {
		return nil, failure("readdir", name, fs.ErrInvalid)
	}
	entries, err := fs.ReadDir(f.source, name)
	if err != nil {
		return nil, failure("readdir", name, err)
	}
	for i, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), Suffix) {
			logical := strings.TrimSuffix(entry.Name(), Suffix)
			entries[i] = dirEntry{DirEntry: entry, fs: f, name: logical, fullName: path.Join(name, logical)}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

type fileInfo struct {
	fs.FileInfo
	name string
	size int64
}

func (i fileInfo) Name() string { return i.name }
func (i fileInfo) Size() int64  { return i.size }

type dirEntry struct {
	fs.DirEntry
	fs             FS
	name, fullName string
}

func (e dirEntry) Name() string               { return e.name }
func (e dirEntry) Info() (fs.FileInfo, error) { return e.fs.Stat(e.fullName) }

type fileReader struct {
	*bytes.Reader
	data   []byte
	info   fs.FileInfo
	closed bool
}

func (f *fileReader) Stat() (fs.FileInfo, error) {
	if f.closed {
		return nil, fs.ErrClosed
	}
	return f.info, nil
}
func (f *fileReader) Read(p []byte) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.Reader.Read(p)
}
func (f *fileReader) ReadAt(p []byte, off int64) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.Reader.ReadAt(p, off)
}
func (f *fileReader) Seek(off int64, whence int) (int64, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.Reader.Seek(off, whence)
}
func (f *fileReader) Close() error {
	f.closed = true
	f.data = nil
	f.Reader = bytes.NewReader(nil)
	return nil
}

type directory struct {
	fs.File
	fs             FS
	name           string
	entries        []fs.DirEntry
	offset         int
	loaded, closed bool
}

func (d *directory) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}
	if !d.loaded {
		var err error
		d.entries, err = d.fs.ReadDir(d.name)
		if err != nil {
			return nil, err
		}
		d.loaded = true
	}
	if n > 0 && d.offset >= len(d.entries) {
		return nil, io.EOF
	}
	end := len(d.entries)
	if n > 0 && n < end-d.offset {
		end = d.offset + n
	}
	result := d.entries[d.offset:end]
	d.offset = end
	return result, nil
}
func (d *directory) Close() error { d.closed = true; return d.File.Close() }

var _ fs.ReadFileFS = FS{}
var _ fs.ReadDirFS = FS{}
var _ fs.StatFS = FS{}
var _ io.ReadSeeker = (*fileReader)(nil)
