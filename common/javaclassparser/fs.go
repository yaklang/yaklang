package javaclassparser

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
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
	jarCache       *utils.SafeMapWithKey[string, *filesys.UnifiedFS]
	recursiveParse bool // 是否递归解析嵌套的jar文件，默认为true
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
	return NewJarFSWithOptions(zipFs, true) // 默认启用递归解析
}

func NewJarFSWithOptions(zipFs *filesys.ZipFS, recursiveParse bool) *JarFS {
	return &JarFS{
		ZipFS:          zipFs,
		jarCache:       utils.NewSafeMapWithKey[string, *filesys.UnifiedFS](),
		recursiveParse: recursiveParse,
	}
}

func decompileClassBytes(name string, data []byte) []byte {
	if !strings.HasSuffix(strings.ToLower(name), ".class") {
		return data
	}
	cf, err := Parse(data)
	if err != nil {
		return []byte(fmt.Sprintf("// decompile parse failed for %s: %v\n", name, err))
	}
	source, err := cf.Dump()
	if err != nil {
		return []byte(fmt.Sprintf("// decompile dump failed for %s: %v\n", name, err))
	}
	return []byte(source)
}

func (z *JarFS) ReadFile(name string) ([]byte, error) {
	if isArchivePath(name) {
		return z.readFileFromJar(name)
	}
	data, err := z.ZipFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return decompileClassBytes(name, data), nil
}

func (f *JarFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.Open(name)
}

func (z *JarFS) Open(name string) (fs.File, error) {
	raw, err := z.ZipFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return memfile.New(decompileClassBytes(name, raw)), nil
}

func (z *JarFS) Stat(name string) (fs.FileInfo, error) {
	if isArchivePath(name) {
		return z.statInJar(name)
	}
	if isArchiveFile(name) {
		info, err := z.ZipFS.Stat(name)
		if err != nil {
			return nil, err
		}
		return &ArchiveFileInfo{
			fs:    info,
			name:  info.Name(),
			isDir: true,
		}, nil
	}
	info, err := z.ZipFS.Stat(name)
	if err != nil {
		return nil, err
	}
	return &ArchiveFileInfo{
		fs:    info,
		name:  info.Name(),
		isDir: info.IsDir(),
	}, nil
}

var (
	archiveFileSuffixes   = []string{".jar", ".war", ".ear", ".par", ".zip"}
	jarLikeArchiveSuffixes = []string{".jar", ".war", ".ear", ".par"}
)

func isArchiveFile(path string) bool {
	path = normalizeArchivePath(path)
	lowerPath := strings.ToLower(path)
	for _, ext := range archiveFileSuffixes {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func isArchivePath(name string) bool {
	name = normalizeArchivePath(name)
	lowerPath := strings.ToLower(name)
	for _, ext := range archiveFileSuffixes {
		if strings.Contains(lowerPath, ext+"/") || strings.Contains(lowerPath, ext+"!") {
			return true
		}
	}
	return false
}

func parseArchivePath(fullPath string) (archivePath, internalPath string, ok bool) {
	fullPath = normalizeArchivePath(fullPath)
	lowerPath := strings.ToLower(fullPath)

	for _, ext := range archiveFileSuffixes {
		if strings.Contains(lowerPath, ext+"/") || strings.Contains(lowerPath, ext+"!") {
			return parseJarOrZipPath(fullPath, ext)
		}
	}
	return "", "", false
}

func isJarLikeArchivePath(path string) bool {
	path = normalizeArchivePath(path)
	lowerPath := strings.ToLower(path)
	for _, ext := range jarLikeArchiveSuffixes {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func parseJarOrZipPath(fullPath, ext string) (archivePath, internalPath string, ok bool) {
	extWithSlash := ext + "/"
	extWithBang := ext + "!"

	idx := strings.Index(fullPath, extWithSlash)
	if idx == -1 {
		bangIdx := strings.Index(fullPath, extWithBang)
		if bangIdx == -1 {
			return "", "", false
		}
		archivePath = fullPath[:bangIdx+len(ext)]
		internalPath = strings.TrimPrefix(fullPath[bangIdx+len(extWithBang):], "/")
		return archivePath, internalPath, true
	}

	archivePath = fullPath[:idx+len(ext)]
	internalPath = strings.TrimPrefix(fullPath[idx+len(extWithSlash):], "/")
	return archivePath, internalPath, true
}

func normalizeArchivePath(name string) string {
	name = strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(name)), "\\", "/")
	if name == "" || name == "." {
		return name
	}
	name = path.Clean(name)
	name = strings.TrimLeft(name, "/")
	if name == "" {
		return "."
	}
	return name
}

func (z *JarFS) getNestedJarFS(jarPath string) (*filesys.UnifiedFS, error) {
	// 如果禁用了递归解析，返回错误
	if !z.recursiveParse {
		return nil, os.ErrNotExist
	}

	if nestedJarFS, ok := z.jarCache.Get(jarPath); ok {
		return nestedJarFS, nil
	}

	jarContent, err := z.ZipFS.ReadFile(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read jar file: %s", jarPath)
	}

	nestedZipFS, err := filesys.NewZipFSRaw(bytes.NewReader(jarContent), int64(len(jarContent)))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for jar: %s", jarPath)
	}

	// 嵌套的jar也继承递归解析设置
	nestedJarFS := NewJarFSWithOptions(nestedZipFS, z.recursiveParse)
	unifiedFS := filesys.NewUnifiedFS(nestedJarFS,
		filesys.WithUnifiedFsExtMap(".class", ".java"),
	)
	z.jarCache.Set(jarPath, unifiedFS)
	return unifiedFS, nil
}

func (z *JarFS) statInJar(name string) (fs.FileInfo, error) {
	jarPath, internalPath, ok := parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	unifiedFS, err := z.getNestedJarFS(jarPath)
	if err != nil {
		return nil, err
	}

	info, err := unifiedFS.Stat(internalPath)
	if err != nil {
		if _, readErr := unifiedFS.ReadDir(internalPath); readErr == nil {
			return &ArchiveFileInfo{
				fs:    info,
				name:  name,
				isDir: true,
			}, nil
		}
		return nil, err
	}

	return &ArchiveFileInfo{
		fs:    info,
		name:  name,
		isDir: info.IsDir(),
	}, nil
}

func (z *JarFS) readDirFromJar(fullPath string) ([]fs.DirEntry, error) {
	jarPath, internalPath, ok := parseArchivePath(fullPath)
	if !ok {
		return nil, os.ErrNotExist
	}

	unifiedFS, err := z.getNestedJarFS(jarPath)
	if err != nil {
		return nil, err
	}

	return unifiedFS.ReadDir(internalPath)
}

func (z *JarFS) readFileFromJar(name string) ([]byte, error) {
	jarPath, internalPath, ok := parseArchivePath(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	unifiedFS, err := z.getNestedJarFS(jarPath)
	if err != nil {
		return nil, err
	}

	return unifiedFS.ReadFile(internalPath)
}

func (z *JarFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if isArchivePath(name) {
		return z.readDirFromJar(name)
	}
	if isArchiveFile(name) {
		info, err := z.ZipFS.Stat(name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			unifiedFS, err := z.getNestedJarFS(name)
			if err != nil {
				return nil, err
			}
			return unifiedFS.ReadDir(".")
		}
	}
	return z.ZipFS.ReadDir(name)
}

type ExpandedZipFS struct {
	underlying       fi.FileSystem
	zipFS            *filesys.ZipFS // optional container (zip root); nil for local directories
	recursiveParse   bool
	jarCache         *utils.SafeMapWithKey[string, *filesys.UnifiedFS]
	zipCache         *utils.SafeMapWithKey[string, *filesys.ZipFS]
}

var _ fi.FileSystem = (*ExpandedZipFS)(nil)

func NewExpandedZipFS(underlying fi.FileSystem, zipFS *filesys.ZipFS) *ExpandedZipFS {
	return NewExpandedZipFSWithOptions(underlying, zipFS, true)
}

func NewExpandedZipFSWithOptions(underlying fi.FileSystem, zipFS *filesys.ZipFS, recursiveParse bool) *ExpandedZipFS {
	return &ExpandedZipFS{
		underlying:       underlying,
		zipFS:            zipFS,
		recursiveParse:   recursiveParse,
		jarCache:         utils.NewSafeMapWithKey[string, *filesys.UnifiedFS](),
		zipCache:         utils.NewSafeMapWithKey[string, *filesys.ZipFS](),
	}
}

func (e *ExpandedZipFS) getArchiveFS(archivePath string) (fi.FileSystem, error) {
	if isJarLikeArchivePath(archivePath) {
		return e.GetJarFS(archivePath)
	}
	if strings.HasSuffix(strings.ToLower(normalizeArchivePath(archivePath)), ".zip") {
		return e.getZipFS(archivePath)
	}
	return nil, os.ErrNotExist
}

func (e *ExpandedZipFS) GetJarFS(jarPath string) (*filesys.UnifiedFS, error) {
	if jarFS, ok := e.jarCache.Get(jarPath); ok {
		return jarFS, nil
	}

	jarContent, err := e.readArchiveBytes(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read jar file: %s", jarPath)
	}

	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(jarContent), int64(len(jarContent)))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for jar: %s", jarPath)
	}

	jarFS := NewJarFSWithOptions(zipFS, e.recursiveParse)
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

	zipContent, err := e.readArchiveBytes(zipPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read zip file: %s", zipPath)
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

	return entries, nil
}

func (e *ExpandedZipFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if isArchivePath(name) {
		return e.readDirFromArchive(name)
	}
	if isArchiveFile(name) {
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
		expandedEntries = append(expandedEntries, entry)
	}
	return expandedEntries, nil
}

func (e *ExpandedZipFS) readDirFromArchive(fullPath string) ([]fs.DirEntry, error) {
	archivePath, internalPath, ok := parseArchivePath(fullPath)
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

	return entries, nil
}

func (e *ExpandedZipFS) Stat(name string) (fs.FileInfo, error) {
	if isArchivePath(name) {
		return e.statInArchive(name)
	}
	if isArchiveFile(name) {
		info, err := e.underlying.Stat(name)
		if err != nil {
			return nil, err
		}
		return &ArchiveFileInfo{
			fs:    info,
			name:  info.Name(),
			isDir: true,
		}, nil
	}
	return e.underlying.Stat(name)
}

func (e *ExpandedZipFS) statInArchive(name string) (fs.FileInfo, error) {
	archivePath, internalPath, ok := parseArchivePath(name)
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
				fs:    info,
				name:  name,
				isDir: true,
			}, nil
		}
		return nil, err
	}

	return &ArchiveFileInfo{
		fs:    info,
		name:  name,
		isDir: info.IsDir(),
	}, nil
}

func (e *ExpandedZipFS) ReadFile(name string) ([]byte, error) {
	if isArchivePath(name) {
		return e.readFileFromArchive(name)
	}
	return e.underlying.ReadFile(name)
}

func (e *ExpandedZipFS) readFileFromArchive(name string) ([]byte, error) {
	archivePath, internalPath, ok := parseArchivePath(name)
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
	if isArchivePath(name) {
		data, err := e.readFileFromArchive(name)
		if err != nil {
			return nil, err
		}
		return memfile.New(data), nil
	}
	return e.underlying.Open(name)
}

func (e *ExpandedZipFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	if isArchivePath(name) {
		return e.Open(name)
	}
	return e.underlying.OpenFile(name, flag, perm)
}

type ArchiveFileInfo struct {
	fs    fs.FileInfo
	name  string
	isDir bool
}

func (a *ArchiveFileInfo) Name() string {
	return a.fs.Name()
}

func (a *ArchiveFileInfo) Size() int64 {
	return a.fs.Size()
}

func (a *ArchiveFileInfo) Mode() fs.FileMode {
	if a.isDir {
		return fs.ModeDir
	}
	return a.fs.Mode()
}

func (a *ArchiveFileInfo) ModTime() time.Time {
	return a.fs.ModTime()
}

func (a *ArchiveFileInfo) IsDir() bool {
	return a.isDir
}

func (a *ArchiveFileInfo) Sys() interface{} {
	return a.fs.Sys()
}
