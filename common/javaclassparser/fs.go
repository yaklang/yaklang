package javaclassparser

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
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
	// fmt.Printf("start parse file: %s\n", name)
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
		// If parsing fails, return raw bytes as fallback
		return memfile.New(raw), nil
	}
	source, err := cf.Dump()
	if err != nil {
		// If dump fails, return raw bytes as fallback
		return memfile.New(raw), nil
	}
	return memfile.New([]byte(source)), nil
}

// ZipJarHelper 处理 ZIP 文件中包含多个 JAR 文件的情况
// 支持从 ZIP 内的 JAR 文件中读取和编译 Java 代码
type ZipJarHelper struct {
	zipFS    *filesys.ZipFS
	jarCache map[string]*JarFS
	jarMutex sync.RWMutex
}

func NewZipJarHelper(zipFS *filesys.ZipFS) *ZipJarHelper {
	return &ZipJarHelper{
		zipFS:    zipFS,
		jarCache: make(map[string]*JarFS),
	}
}

// ParseJarPath 解析包含 JAR 路径的完整路径
// 例如: "lib/app.jar/com/example/Class.java" -> ("lib/app.jar", "com/example/Class.java", true)
func (h *ZipJarHelper) ParseJarPath(fullPath string) (string, string, bool) {
	jarIdx := strings.Index(fullPath, ".jar/")
	if jarIdx == -1 {
		jarIdx = strings.Index(fullPath, ".jar!")
		if jarIdx == -1 {
			return "", "", false
		}
		jarIdx += 4
	} else {
		jarIdx += 5
	}

	jarPath := fullPath[:jarIdx]
	internalPath := strings.TrimPrefix(fullPath[jarIdx:], "/")

	return jarPath, internalPath, true
}

// GetJarFS 获取或创建 JAR 文件系统（带缓存）
func (h *ZipJarHelper) GetJarFS(jarPath string) (*JarFS, error) {
	h.jarMutex.RLock()
	if fs, ok := h.jarCache[jarPath]; ok {
		h.jarMutex.RUnlock()
		return fs, nil
	}
	h.jarMutex.RUnlock()

	jarContent, err := h.zipFS.ReadFile(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read jar file from zip: %s", jarPath)
	}

	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(jarContent), int64(len(jarContent)))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for jar: %s", jarPath)
	}

	jarFS := NewJarFS(zipFS)

	h.jarMutex.Lock()
	h.jarCache[jarPath] = jarFS
	h.jarMutex.Unlock()

	return jarFS, nil
}

// ReadFileFromJar 从 ZIP 内的 JAR 文件中读取文件内容
func (h *ZipJarHelper) ReadFileFromJar(jarPath, internalPath string) ([]byte, error) {
	jarFS, err := h.GetJarFS(jarPath)
	if err != nil {
		return nil, err
	}
	return jarFS.ReadFile(internalPath)
}
