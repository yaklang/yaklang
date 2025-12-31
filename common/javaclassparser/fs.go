package javaclassparser

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memfile"
)

type JarFS struct {
	*filesys.ZipFS
}

var _ fs.FS = (*JarFS)(nil)
var _ fs.ReadFileFS = (*JarFS)(nil)
var _ fs.ReadDirFS = (*JarFS)(nil)

func NewJarFSFromLocal(path string) (*JarFS, error) {
	zipFS, err := filesys.NewZipFSFromLocal(path)
	if err != nil {
		return nil, err
	}
	return NewJarFS(zipFS), nil
}

func NewJarFS(zipFs *filesys.ZipFS) *JarFS {
	return &JarFS{
		ZipFS: zipFs,
	}
}

func (z *JarFS) ReadFile(name string) ([]byte, error) {
	data, err := z.ZipFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	cf, err := Parse(data)
	if err != nil {
		return data, nil
	}
	source, err := cf.Dump()
	if err != nil {
		return data, nil
	}
	return []byte(source), nil
}

func (f *JarFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.Open(name)
}

func (z *JarFS) Open(name string) (fs.File, error) {
	raw, err := z.ZipFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	cf, err := Parse(raw)
	if err != nil {
		return memfile.New(raw), nil
	}
	source, err := cf.Dump()
	if err != nil {
		return memfile.New(raw), nil
	}
	return memfile.New([]byte(source)), nil
}

type ExpandedZipFS struct {
	underlying fi.FileSystem
	zipFS      *filesys.ZipFS
	jarCache   *utils.SafeMapWithKey[string, *filesys.UnifiedFS]
	zipCache   *utils.SafeMapWithKey[string, *filesys.ZipFS]
}

var _ fi.FileSystem = (*ExpandedZipFS)(nil)

func NewExpandedZipFS(underlying fi.FileSystem, zipFS *filesys.ZipFS) *ExpandedZipFS {
	return &ExpandedZipFS{
		underlying: underlying,
		zipFS:      zipFS,
		jarCache:   utils.NewSafeMapWithKey[string, *filesys.UnifiedFS](),
		zipCache:   utils.NewSafeMapWithKey[string, *filesys.ZipFS](),
	}
}

func (e *ExpandedZipFS) isArchiveFile(path string) bool {
	lowerPath := strings.ToLower(path)
	return strings.HasSuffix(lowerPath, ".jar") ||
		strings.HasSuffix(lowerPath, ".war") ||
		strings.HasSuffix(lowerPath, ".zip")
}

func (e *ExpandedZipFS) isArchivePath(name string) bool {
	return strings.Contains(name, ".jar/") || strings.Contains(name, ".jar!") ||
		strings.Contains(name, ".zip/") || strings.Contains(name, ".zip!")
}

func (e *ExpandedZipFS) parseArchivePath(fullPath string) (archivePath, internalPath string, ok bool) {
	lowerPath := strings.ToLower(fullPath)

	if strings.Contains(lowerPath, ".jar/") || strings.Contains(lowerPath, ".jar!") {
		return e.parseJarOrZipPath(fullPath, ".jar")
	}
	if strings.Contains(lowerPath, ".zip/") || strings.Contains(lowerPath, ".zip!") {
		return e.parseJarOrZipPath(fullPath, ".zip")
	}
	return "", "", false
}

func (e *ExpandedZipFS) parseJarOrZipPath(fullPath, ext string) (archivePath, internalPath string, ok bool) {
	extWithSlash := ext + "/"
	extWithBang := ext + "!"

	idx := strings.Index(fullPath, extWithSlash)
	if idx == -1 {
		idx = strings.Index(fullPath, extWithBang)
		if idx == -1 {
			return "", "", false
		}
		idx += len(ext)
	} else {
		idx += len(extWithSlash)
	}

	archivePath = fullPath[:idx]
	internalPath = strings.TrimPrefix(fullPath[idx:], "/")
	return archivePath, internalPath, true
}

func (e *ExpandedZipFS) getArchiveFS(archivePath string) (fi.FileSystem, error) {
	lowerPath := strings.ToLower(archivePath)

	if strings.Contains(lowerPath, ".jar") || strings.Contains(lowerPath, ".war") {
		return e.GetJarFS(archivePath)
	}
	if strings.Contains(lowerPath, ".zip") {
		return e.getZipFS(archivePath)
	}
	return nil, os.ErrNotExist
}

func (e *ExpandedZipFS) GetJarFS(jarPath string) (*filesys.UnifiedFS, error) {
	if jarFS, ok := e.jarCache.Get(jarPath); ok {
		return jarFS, nil
	}

	jarContent, err := e.zipFS.ReadFile(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read jar file from zip: %s", jarPath)
	}

	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(jarContent), int64(len(jarContent)))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for jar: %s", jarPath)
	}

	jarFS := NewJarFS(zipFS)
	undiFS := filesys.NewUnifiedFS(jarFS,
		filesys.WithUnifiedFsExtMap(".class", ".java"),
	)
	e.jarCache.Set(jarPath, undiFS)
	return undiFS, nil
}

func (e *ExpandedZipFS) getZipFS(zipPath string) (*filesys.ZipFS, error) {
	if zipFS, ok := e.zipCache.Get(zipPath); ok {
		return zipFS, nil
	}

	zipContent, err := e.zipFS.ReadFile(zipPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read zip file from zip: %s", zipPath)
	}

	nestedZipFS, err := filesys.NewZipFSRaw(bytes.NewReader(zipContent), int64(len(zipContent)))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for nested zip: %s", zipPath)
	}

	e.zipCache.Set(zipPath, nestedZipFS)
	return nestedZipFS, nil
}

func (e *ExpandedZipFS) expandArchive(archivePath string) ([]fs.DirEntry, error) {
	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	entries, err := archiveFS.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var result []fs.DirEntry
	for _, entry := range entries {
		result = append(result, &ArchiveDirEntry{
			name:     entry.Name(),
			isDir:    entry.IsDir(),
			original: entry,
		})
	}
	return result, nil
}

func (e *ExpandedZipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if e.isArchivePath(name) {
		return e.readDirFromArchive(name)
	}
	if e.isArchiveFile(name) {
		_, err := e.underlying.Stat(name)
		if err != nil {
			return nil, err
		}
		return e.expandArchive(name)
	}
	entries, err := e.underlying.ReadDir(name)
	if err != nil {
		return nil, err
	}
	var expandedEntries []fs.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			expandedEntries = append(expandedEntries, entry)
			continue
		}
		entryName := entry.Name()
		if e.isArchiveFile(entryName) {
			expandedEntries = append(expandedEntries, &ArchiveDirEntry{
				name:     entryName,
				isDir:    true,
				original: entry,
			})
		} else {
			expandedEntries = append(expandedEntries, entry)
		}
	}
	return expandedEntries, nil
}

func (e *ExpandedZipFS) readDirFromArchive(fullPath string) ([]fs.DirEntry, error) {
	archivePath, internalPath, ok := e.parseArchivePath(fullPath)
	if !ok {
		return nil, os.ErrNotExist
	}

	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	entries, err := archiveFS.ReadDir(internalPath)
	if err != nil {
		return nil, err
	}

	var result []fs.DirEntry
	for _, entry := range entries {
		result = append(result, &ArchiveDirEntry{
			name:     entry.Name(),
			isDir:    entry.IsDir(),
			original: entry,
		})
	}
	return result, nil
}

func (e *ExpandedZipFS) Stat(name string) (fs.FileInfo, error) {
	if e.isArchivePath(name) {
		return e.statInArchive(name)
	}
	if e.isArchiveFile(name) {
		info, err := e.underlying.Stat(name)
		if err != nil {
			return nil, err
		}
		return &ArchiveFileInfo{
			name:  info.Name(),
			isDir: true,
		}, nil
	}
	return e.underlying.Stat(name)
}

func (e *ExpandedZipFS) statInArchive(name string) (fs.FileInfo, error) {
	archivePath, internalPath, ok := e.parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	info, err := archiveFS.Stat(internalPath)
	if err != nil {
		if _, readErr := archiveFS.ReadDir(internalPath); readErr == nil {
			return &ArchiveFileInfo{
				name:  name,
				isDir: true,
			}, nil
		}
		return nil, err
	}

	return &ArchiveFileInfo{
		name:  name,
		isDir: info.IsDir(),
	}, nil
}

func (e *ExpandedZipFS) ReadFile(name string) ([]byte, error) {
	if e.isArchivePath(name) {
		return e.readFileFromArchive(name)
	}
	return e.underlying.ReadFile(name)
}

func (e *ExpandedZipFS) readFileFromArchive(name string) ([]byte, error) {
	archivePath, internalPath, ok := e.parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	archiveFS, err := e.getArchiveFS(archivePath)
	if err != nil {
		return nil, err
	}

	if jarFS, ok := archiveFS.(*filesys.UnifiedFS); ok {
		return jarFS.ReadFile(internalPath)
	}
	if zipFS, ok := archiveFS.(*filesys.ZipFS); ok {
		return zipFS.ReadFile(internalPath)
	}
	return nil, os.ErrNotExist
}

func (e *ExpandedZipFS) Join(elem ...string) string {
	return e.underlying.Join(elem...)
}

func (e *ExpandedZipFS) GetSeparators() rune {
	return e.underlying.GetSeparators()
}

func (e *ExpandedZipFS) IsAbs(name string) bool {
	return e.underlying.IsAbs(name)
}

func (e *ExpandedZipFS) Getwd() (string, error) {
	return e.underlying.Getwd()
}

func (e *ExpandedZipFS) Exists(path string) (bool, error) {
	return e.underlying.Exists(path)
}

func (e *ExpandedZipFS) Rename(old string, new string) error {
	return e.underlying.Rename(old, new)
}

func (e *ExpandedZipFS) Rel(base string, target string) (string, error) {
	return e.underlying.Rel(base, target)
}

func (e *ExpandedZipFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return e.underlying.WriteFile(name, data, perm)
}

func (e *ExpandedZipFS) Delete(name string) error {
	return e.underlying.Delete(name)
}

func (e *ExpandedZipFS) MkdirAll(name string, perm os.FileMode) error {
	return e.underlying.MkdirAll(name, perm)
}

func (e *ExpandedZipFS) ExtraInfo(path string) map[string]any {
	return e.underlying.ExtraInfo(path)
}

func (e *ExpandedZipFS) Base(p string) string {
	return e.underlying.Base(p)
}

func (e *ExpandedZipFS) PathSplit(s string) (string, string) {
	return e.underlying.PathSplit(s)
}

func (e *ExpandedZipFS) Ext(s string) string {
	return e.underlying.Ext(s)
}

func (e *ExpandedZipFS) Open(name string) (fs.File, error) {
	return e.underlying.Open(name)
}

func (e *ExpandedZipFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return e.underlying.OpenFile(name, flag, perm)
}

type ArchiveDirEntry struct {
	name     string
	isDir    bool
	original fs.DirEntry
}

func (a *ArchiveDirEntry) Name() string {
	return a.name
}

func (a *ArchiveDirEntry) IsDir() bool {
	return a.isDir
}

func (a *ArchiveDirEntry) Type() fs.FileMode {
	if a.isDir {
		return fs.ModeDir
	}
	return 0
}

func (a *ArchiveDirEntry) Info() (fs.FileInfo, error) {
	if a.original != nil {
		return a.original.Info()
	}
	return &ArchiveFileInfo{
		name:  a.name,
		isDir: a.isDir,
	}, nil
}

type ArchiveFileInfo struct {
	name  string
	isDir bool
}

func (a *ArchiveFileInfo) Name() string {
	return filepath.Base(a.name)
}

func (a *ArchiveFileInfo) Size() int64 {
	return 0
}

func (a *ArchiveFileInfo) Mode() fs.FileMode {
	if a.isDir {
		return fs.ModeDir
	}
	return 0
}

func (a *ArchiveFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (a *ArchiveFileInfo) IsDir() bool {
	return a.isDir
}

func (a *ArchiveFileInfo) Sys() interface{} {
	return nil
}
