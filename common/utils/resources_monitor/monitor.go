package resources_monitor

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

// ResourceMonitor provides unified access to embedded resources,
// handling both standard embed.FS and gzip-compressed tarballs transparently.
// It acts as a wrapper around filesys_interface.FileSystem with additional
// capabilities for hash calculation and lazy loading notifications.
type ResourceMonitor interface {
	filesys_interface.FileSystem
	GetHash() (string, error)
	SetNotify(func(float64, string))
}

// standardResourceMonitor implements ResourceMonitor for standard embed.FS
type standardResourceMonitor struct {
	filesys_interface.FileSystem
	rawFS   embed.FS
	hashExt string
}

func NewStandardResourceMonitor(fs embed.FS, hashExt string) ResourceMonitor {
	return &standardResourceMonitor{
		FileSystem: filesys.NewEmbedFS(fs),
		rawFS:      fs,
		hashExt:    hashExt,
	}
}

func (s *standardResourceMonitor) GetHash() (string, error) {
	if s.hashExt != "" {
		return filesys.CreateEmbedFSHash(s.rawFS, filesys.WithIncludeExts(s.hashExt))
	}
	return filesys.CreateEmbedFSHash(s.rawFS)
}

func (s *standardResourceMonitor) SetNotify(f func(float64, string)) {}

// gzipResourceMonitor implements ResourceMonitor for gzip embed.FS.
// pathPrefix: when non-empty (e.g. "base-yak-plugin"), enables fallback so both --root-path
// tar (paths "prefix/...") and non-root-path tar (paths "...") work. Builds should use --root-path.
type gzipResourceMonitor struct {
	rawFS      *embed.FS
	fileName   string
	pathPrefix string // optional root dir name for fallback; empty = no fallback
	notify     func(float64, string)

	initOnce sync.Once
	fs       filesys_interface.FileSystem
}

// NewGzipResourceMonitor creates a ResourceMonitor for gzip-compressed tar embed.
// pathPrefix: root directory name when tar is built with --root-path (e.g. "base-yak-plugin", "buildinforge", "buildin").
// When set, failed lookups for "pathPrefix/rest" are retried as "rest" for compatibility with tar built without --root-path.
// Pass "" when no directory prefix is used.
func NewGzipResourceMonitor(fs *embed.FS, fileName string, pathPrefix string) ResourceMonitor {
	return &gzipResourceMonitor{
		rawFS:      fs,
		fileName:   fileName,
		pathPrefix: strings.TrimSuffix(pathPrefix, "/"),
		fs:         nil,
	}
}

func (g *gzipResourceMonitor) stripPrefix(name string) (fallback string, ok bool) {
	if g.pathPrefix == "" {
		return "", false
	}
	name = path.Clean(name)
	if name == g.pathPrefix {
		return ".", true
	}
	prefixSlash := g.pathPrefix + "/"
	if strings.HasPrefix(name, prefixSlash) {
		return strings.TrimPrefix(name, prefixSlash), true
	}
	return "", false
}

func (g *gzipResourceMonitor) SetNotify(f func(float64, string)) {
	g.notify = f
}

// ensureInit initializes the filesystem if it hasn't been already
func (g *gzipResourceMonitor) ensureInit() {
	g.initOnce.Do(func() {
		if g.notify != nil {
			g.notify(0, "正在解压资源文件...")
		}
		fs, err := gzip_embed.NewPreprocessingEmbed(g.rawFS, g.fileName, true)
		if err != nil {
			log.Errorf("init gzip embed[%s] failed: %v", g.fileName, err)
			g.fs = gzip_embed.NewEmptyPreprocessingEmbed()
			return
		}
		g.fs = fs
		if g.notify != nil {
			g.notify(0.05, "资源文件解压完成")
		}
	})
}

func (g *gzipResourceMonitor) GetHash() (string, error) {
	// For gzip embed, we just calculate the hash of the embedded tar.gz file
	return filesys.CreateEmbedFSHash(*g.rawFS)
}

// Delegate all FileSystem methods to the underlying fs instance

func (g *gzipResourceMonitor) ReadDir(name string) ([]fs.DirEntry, error) {
	g.ensureInit()
	entries, err := g.fs.ReadDir(name)
	if err == nil {
		return entries, nil
	}
	if fallback, ok := g.stripPrefix(name); ok {
		return g.fs.ReadDir(fallback)
	}
	return nil, err
}

func (g *gzipResourceMonitor) ReadFile(name string) ([]byte, error) {
	g.ensureInit()
	data, err := g.fs.ReadFile(name)
	if err == nil {
		return data, nil
	}
	if fallback, ok := g.stripPrefix(name); ok {
		return g.fs.ReadFile(fallback)
	}
	return nil, err
}

func (g *gzipResourceMonitor) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	g.ensureInit()
	f, err := g.fs.OpenFile(name, flag, perm)
	if err == nil {
		return f, nil
	}
	if fallback, ok := g.stripPrefix(name); ok {
		return g.fs.OpenFile(fallback, flag, perm)
	}
	return nil, err
}

func (g *gzipResourceMonitor) Open(name string) (fs.File, error) {
	g.ensureInit()
	f, err := g.fs.Open(name)
	if err == nil {
		return f, nil
	}
	if fallback, ok := g.stripPrefix(name); ok {
		return g.fs.Open(fallback)
	}
	return nil, err
}

func (g *gzipResourceMonitor) Stat(name string) (fs.FileInfo, error) {
	g.ensureInit()
	info, err := g.fs.Stat(name)
	if err == nil {
		return info, nil
	}
	if fallback, ok := g.stripPrefix(name); ok {
		return g.fs.Stat(fallback)
	}
	return nil, err
}

func (g *gzipResourceMonitor) ExtraInfo(name string) map[string]any {
	g.ensureInit()
	return g.fs.ExtraInfo(name)
}

func (g *gzipResourceMonitor) GetSeparators() rune {
	g.ensureInit()
	return g.fs.GetSeparators()
}

func (g *gzipResourceMonitor) Join(elem ...string) string {
	g.ensureInit()
	return g.fs.Join(elem...)
}

func (g *gzipResourceMonitor) Base(path string) string {
	g.ensureInit()
	return g.fs.Base(path)
}

func (g *gzipResourceMonitor) PathSplit(path string) (string, string) {
	g.ensureInit()
	return g.fs.PathSplit(path)
}

func (g *gzipResourceMonitor) Ext(path string) string {
	g.ensureInit()
	return g.fs.Ext(path)
}

func (g *gzipResourceMonitor) IsAbs(path string) bool {
	g.ensureInit()
	return g.fs.IsAbs(path)
}

func (g *gzipResourceMonitor) Getwd() (string, error) {
	g.ensureInit()
	return g.fs.Getwd()
}

func (g *gzipResourceMonitor) Exists(name string) (bool, error) {
	g.ensureInit()
	exists, err := g.fs.Exists(name)
	if err == nil && exists {
		return true, nil
	}
	if fallback, ok := g.stripPrefix(name); ok {
		return g.fs.Exists(fallback)
	}
	return false, err
}

func (g *gzipResourceMonitor) Rel(basepath, targpath string) (string, error) {
	g.ensureInit()
	return g.fs.Rel(basepath, targpath)
}

func (g *gzipResourceMonitor) Rename(oldname, newname string) error {
	g.ensureInit()
	return g.fs.Rename(oldname, newname)
}

func (g *gzipResourceMonitor) WriteFile(filename string, data []byte, perm os.FileMode) error {
	g.ensureInit()
	return g.fs.WriteFile(filename, data, perm)
}

func (g *gzipResourceMonitor) Delete(filename string) error {
	g.ensureInit()
	return g.fs.Delete(filename)
}

func (g *gzipResourceMonitor) MkdirAll(path string, perm os.FileMode) error {
	g.ensureInit()
	return g.fs.MkdirAll(path, perm)
}
