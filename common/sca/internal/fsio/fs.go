// Package fsio is a read-only adapter for explicitly supplied snapshots. The
// caller owns snapshot consistency; io/fs alone is not a mutable-host sandbox.
package fsio

import (
	"context"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"io/fs"
	"os"
	"path"
	"strings"
)

type Snapshot struct{ source fs.FS }

func New(source fs.FS) *Snapshot { return &Snapshot{source: source} }
func valid(name string) bool     { return fs.ValidPath(name) && !strings.ContainsAny(name, `\:`) }
func (s *Snapshot) Open(name string) (fs.File, error) {
	if s.source == nil || !valid(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return s.source.Open(name)
}
func (s *Snapshot) ReadDir(name string) ([]fs.DirEntry, error) {
	if s.source == nil || !valid(name) {
		return nil, fs.ErrInvalid
	}
	return fs.ReadDir(s.source, name)
}

func (s *Snapshot) ReadFile(name string) ([]byte, error) {
	f, e := s.Open(name)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	ctx := context.Background()
	if c, ok := f.(interface{ Context() context.Context }); ok {
		ctx = c.Context()
	}
	return textdecode.ReadRaw(ctx, f, 16<<20)
}
func (s *Snapshot) Stat(name string) (fs.FileInfo, error) {
	if s.source == nil || !valid(name) {
		return nil, fs.ErrInvalid
	}
	return fs.Stat(s.source, name)
}

func (s *Snapshot) OpenFile(name string, flag int, _ os.FileMode) (fs.File, error) {
	if flag != os.O_RDONLY {
		return nil, fs.ErrPermission
	}
	return s.Open(name)
}
func (*Snapshot) ExtraInfo(string) map[string]any     { return nil }
func (*Snapshot) GetSeparators() rune                 { return '/' }
func (*Snapshot) Join(e ...string) string             { return path.Join(e...) }
func (*Snapshot) Base(p string) string                { return path.Base(p) }
func (*Snapshot) PathSplit(p string) (string, string) { return path.Split(p) }
func (*Snapshot) Ext(p string) string                 { return path.Ext(p) }
func (*Snapshot) IsAbs(p string) bool                 { return path.IsAbs(p) }
func (*Snapshot) Getwd() (string, error)              { return ".", nil }
func (s *Snapshot) Exists(p string) (bool, error) {
	_, e := s.Stat(p)
	if os.IsNotExist(e) {
		return false, nil
	}
	return e == nil, e
}
func (*Snapshot) Rel(base, target string) (string, error) {
	if !valid(base) || !valid(target) {
		return "", fs.ErrInvalid
	}
	if base == "." {
		return target, nil
	}
	prefix := base + "/"
	if strings.HasPrefix(target, prefix) {
		return strings.TrimPrefix(target, prefix), nil
	}
	return "", fs.ErrPermission
}
func (*Snapshot) Rename(string, string) error                 { return fs.ErrPermission }
func (*Snapshot) WriteFile(string, []byte, os.FileMode) error { return fs.ErrPermission }
func (*Snapshot) Delete(string) error                         { return fs.ErrPermission }
func (*Snapshot) MkdirAll(string, os.FileMode) error          { return fs.ErrPermission }
