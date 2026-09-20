package gzip_embed

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// PreprocessingEmbed reads a tar.gz embedded by the release build. Uncached
// readers decompress on demand; development builds can keep using embed.FS.
type PreprocessingEmbed struct {
	*embed.FS
	EnableCache    bool
	cacheFile      map[string][]byte
	sourceFileName string
	cachedHash     string // 缓存的哈希值
	decode         DecodeFunc
}

// DecodeFunc 在读取嵌入式 tar.gz 原始字节后、解压前执行，用于支持 XOR/自定义编码。
type DecodeFunc func([]byte) ([]byte, error)

func NewEmptyPreprocessingEmbed() *PreprocessingEmbed {
	return &PreprocessingEmbed{
		cacheFile: map[string][]byte{},
	}
}

// NewPreprocessingEmbed create a CompressFS instance
// fs is embed.FS instance, compressDirs is a map, key is virtual dir, value is compress file name
func NewPreprocessingEmbed(fs *embed.FS, fileName string, cache bool) (*PreprocessingEmbed, error) {
	return NewPreprocessingEmbedWithDecode(fs, fileName, cache, nil)
}

// NewPreprocessingEmbedWithDecode 允许对嵌入的压缩资源做自定义解码（例如 XOR）。
func NewPreprocessingEmbedWithDecode(fs *embed.FS, fileName string, cache bool, decode DecodeFunc) (*PreprocessingEmbed, error) {
	cfs := &PreprocessingEmbed{
		FS:             fs,
		cacheFile:      map[string][]byte{},
		sourceFileName: fileName,
		EnableCache:    cache,
		decode:         decode,
	}
	if cache {
		err := cfs.scanFile(func(header *tar.Header, reader io.Reader) (error, bool) {
			if header.Typeflag == tar.TypeDir {
				return nil, true
			}
			buf := &bytes.Buffer{}
			if _, err := io.Copy(buf, reader); err != nil {
				return err, true
			}
			cfs.cacheFile[header.Name] = buf.Bytes()
			return nil, true
		})
		if err != nil {
			return nil, err
		}
	}
	return cfs, nil
}

// NewPreprocessingEmbedWithXORKey 使用固定 XOR key 解码嵌入的压缩资源。
func NewPreprocessingEmbedWithXORKey(fs *embed.FS, fileName string, cache bool, key []byte) (*PreprocessingEmbed, error) {
	return NewPreprocessingEmbedWithDecode(fs, fileName, cache, func(raw []byte) ([]byte, error) {
		if len(key) == 0 {
			return raw, nil
		}
		out := make([]byte, len(raw))
		for i, b := range raw {
			out[i] = b ^ key[i%len(key)]
		}
		return out, nil
	})
}

func (c *PreprocessingEmbed) scanFile(h func(header *tar.Header, reader io.Reader) (error, bool)) error {
	fp, err := c.openSource()
	if err != nil {
		return utils.Errorf("open file %s failed: %v", c.sourceFileName, err)
	}
	defer fp.Close()
	gzReader, err := gzip.NewReader(fp)
	if err != nil {
		return err
	}
	defer gzReader.Close()
	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			// Validate the gzip trailer after the tar end marker.
			_, err = io.Copy(io.Discard, gzReader)
			return err
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir, tar.TypeReg:
			err, ok := h(header, tarReader)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
	}
}

func (c *PreprocessingEmbed) openSource() (io.ReadCloser, error) {
	if c.FS == nil {
		return nil, fs.ErrNotExist
	}
	fp, err := c.FS.Open(c.sourceFileName)
	if err != nil {
		return nil, err
	}
	if c.decode == nil {
		return fp, nil
	}
	defer fp.Close()
	raw, err := io.ReadAll(fp)
	if err != nil {
		return nil, err
	}
	data, err := c.decode(raw)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// ReadFile override embed.FS.ReadFile, if file is compress file, return decompress data
func (c *PreprocessingEmbed) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	var successful bool
	var content []byte
	if c.EnableCache {
		if c.cacheFile == nil {
			return nil, utils.Errorf("cacheFile is nil")
		}
		if data, ok := c.cacheFile[name]; ok {
			successful = true
			content = bytes.Clone(data)
		}
	} else {
		err := c.scanFile(func(header *tar.Header, reader io.Reader) (error, bool) {
			if header.Typeflag == tar.TypeReg && header.Name == name {
				buf := &bytes.Buffer{}
				if _, err := io.Copy(buf, reader); err != nil {
					return err, true
				}
				successful = true
				content = buf.Bytes()
				return nil, false
			}
			return nil, true
		})
		if err != nil {
			return nil, err
		}
	}
	if successful {
		return content, nil
	}
	return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
}

// Verify PreprocessingEmbed implements fi.FileSystem interface
var _ fi.FileSystem = (*PreprocessingEmbed)(nil)

// Open opens the named file
func (c *PreprocessingEmbed) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	data, err := c.ReadFile(name)
	if err == nil {
		return &virtualFile{name: path.Base(name), data: data, reader: bytes.NewReader(data)}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	info, err := c.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	entries, err := c.ReadDir(name)
	if err != nil {
		return nil, err
	}
	return &virtualDir{info: info, entries: entries}, nil
}

// OpenFile opens the named file with specified flag (readonly for tar.gz)
func (c *PreprocessingEmbed) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return c.Open(name)
}

// Stat returns file info for the named file or directory
func (c *PreprocessingEmbed) Stat(name string) (fs.FileInfo, error) {
	// 规范化路径
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimSuffix(name, "/")

	if name != "" && !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}

	// 特殊处理根目录
	if name == "" || name == "." {
		return &virtualFileInfo{
			name:  ".",
			size:  0,
			isDir: true,
		}, nil
	}

	var info fs.FileInfo
	var found bool
	var isDir bool

	// 检查是否存在匹配的目录或文件
	err := c.scanFile(func(header *tar.Header, reader io.Reader) (error, bool) {
		headerName := strings.TrimSuffix(header.Name, "/")

		// 精确匹配文件
		if headerName == name {
			info = archiveFileInfo(header)
			found = true
			return nil, false
		}

		// 检查是否是目录的子项
		if strings.HasPrefix(header.Name, name+"/") {
			// 找到了以 name/ 开头的文件，说明 name 是一个目录
			if !found {
				isDir = true
				found = true
			}
		}

		return nil, true
	})

	if err != nil {
		return nil, err
	}

	if !found {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}

	// 如果找到的是目录但没有直接的 header，创建虚拟目录信息
	if isDir && info == nil {
		return &virtualFileInfo{
			name:  path.Base(name),
			size:  0,
			isDir: true,
		}, nil
	}

	return info, nil
}

// ReadDir reads the directory and returns directory entries
func (c *PreprocessingEmbed) ReadDir(dirname string) ([]fs.DirEntry, error) {
	info, err := c.Stat(dirname)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &fs.PathError{Op: "readdir", Path: dirname, Err: fs.ErrInvalid}
	}
	dirname = strings.TrimSuffix(strings.TrimPrefix(dirname, "./"), "/")
	if dirname == "." {
		dirname = ""
	}
	prefix := dirname
	if prefix != "" {
		prefix += "/"
	}
	entries := map[string]fs.DirEntry{}
	err = c.scanFile(func(header *tar.Header, reader io.Reader) (error, bool) {
		name := strings.TrimSuffix(header.Name, "/")
		if !strings.HasPrefix(name, prefix) {
			return nil, true
		}
		rel := strings.TrimPrefix(name, prefix)
		if rel == "" {
			return nil, true
		}
		first, _, hasChild := strings.Cut(rel, "/")
		if _, exists := entries[first]; !exists {
			info := archiveFileInfo(header)
			if hasChild {
				info = &virtualFileInfo{name: first, isDir: true}
			}
			entries[first] = fs.FileInfoToDirEntry(info)
		}
		return nil, true
	})
	if err != nil {
		return nil, err
	}
	result := make([]fs.DirEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name() < result[j].Name() })
	return result, nil
}

func archiveFileInfo(header *tar.Header) fs.FileInfo {
	return &virtualFileInfo{name: path.Base(strings.TrimSuffix(header.Name, "/")), size: header.Size, isDir: header.Typeflag == tar.TypeDir}
}

// ExtraInfo returns extra information about the fs
func (c *PreprocessingEmbed) ExtraInfo(name string) map[string]any {
	return map[string]any{
		"type":        "gzip_embed",
		"source_file": c.sourceFileName,
		"cache":       c.EnableCache,
	}
}

// PathFileSystem methods
func (c *PreprocessingEmbed) GetSeparators() rune        { return '/' }
func (c *PreprocessingEmbed) Join(elem ...string) string { return path.Join(elem...) }
func (c *PreprocessingEmbed) Base(name string) string    { return path.Base(name) }
func (c *PreprocessingEmbed) PathSplit(name string) (string, string) {
	dir, file := path.Split(name)
	return dir, file
}
func (c *PreprocessingEmbed) Ext(name string) string { return path.Ext(name) }
func (c *PreprocessingEmbed) IsAbs(name string) bool { return len(name) > 0 && name[0] == '/' }
func (c *PreprocessingEmbed) Getwd() (string, error) { return ".", nil }
func (c *PreprocessingEmbed) Exists(name string) (bool, error) {
	_, err := c.ReadFile(name)
	return err == nil, err
}
func (c *PreprocessingEmbed) Rel(basepath, targpath string) (string, error) {
	// Simple implementation for embedded fs
	if strings.HasPrefix(targpath, basepath) {
		return strings.TrimPrefix(targpath, basepath), nil
	}
	return "", errors.New("cannot make relative path")
}

// WriteFileSystem methods (read-only, return errors)
func (c *PreprocessingEmbed) Rename(oldname, newname string) error {
	return errors.New("rename not supported in read-only gzip_embed filesystem")
}
func (c *PreprocessingEmbed) WriteFile(name string, data []byte, perm os.FileMode) error {
	return errors.New("write not supported in read-only gzip_embed filesystem")
}
func (c *PreprocessingEmbed) Delete(name string) error {
	return errors.New("delete not supported in read-only gzip_embed filesystem")
}
func (c *PreprocessingEmbed) MkdirAll(path string, perm os.FileMode) error {
	return errors.New("mkdir not supported in read-only gzip_embed filesystem")
}

// virtualFile implements fs.File interface
type virtualFile struct {
	name   string
	data   []byte
	reader *bytes.Reader
	closed bool
}

func (f *virtualFile) Stat() (fs.FileInfo, error) {
	if f.closed {
		return nil, fs.ErrClosed
	}
	return &virtualFileInfo{
		name: f.name,
		size: int64(len(f.data)),
	}, nil
}

func (f *virtualFile) Read(p []byte) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.Read(p)
}

func (f *virtualFile) Close() error {
	f.closed = true
	f.data = nil
	f.reader = nil
	return nil
}

// Seek and ReadAt let http.FileServer serve byte ranges from an opened asset.
func (f *virtualFile) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.Seek(offset, whence)
}
func (f *virtualFile) ReadAt(p []byte, offset int64) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.ReadAt(p, offset)
}

type virtualDir struct {
	info    fs.FileInfo
	entries []fs.DirEntry
	offset  int
	closed  bool
}

func (d *virtualDir) Stat() (fs.FileInfo, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}
	return d.info, nil
}
func (d *virtualDir) Read([]byte) (int, error) {
	if d.closed {
		return 0, fs.ErrClosed
	}
	return 0, fs.ErrInvalid
}
func (d *virtualDir) Close() error { d.closed = true; d.entries = nil; return nil }
func (d *virtualDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}
	if n > 0 && d.offset >= len(d.entries) {
		return nil, io.EOF
	}
	end := len(d.entries)
	if n > 0 && n < end-d.offset {
		end = d.offset + n
	}
	entries := d.entries[d.offset:end]
	d.offset = end
	return entries, nil
}

// virtualFileInfo implements fs.FileInfo interface
type virtualFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (i *virtualFileInfo) Name() string { return i.name }
func (i *virtualFileInfo) Size() int64  { return i.size }
func (i *virtualFileInfo) Mode() fs.FileMode {
	if i.isDir {
		return fs.ModeDir | 0755
	}
	return 0444
}
func (i *virtualFileInfo) ModTime() time.Time { return time.Time{} }
func (i *virtualFileInfo) IsDir() bool        { return i.isDir }
func (i *virtualFileInfo) Sys() any           { return nil }

// GetHash 计算所有文件内容的哈希值，用于检测文件是否有变动
// 返回一个 SHA256 哈希字符串
func (c *PreprocessingEmbed) GetHash() (string, error) {
	// 如果已经缓存了哈希值，直接返回
	if c.cachedHash != "" {
		return c.cachedHash, nil
	}

	var hashes []string
	// 扫描所有文件并计算每个文件的哈希值
	err := c.scanFile(func(header *tar.Header, reader io.Reader) (error, bool) {
		if header.Typeflag == tar.TypeReg {
			buf := &bytes.Buffer{}
			if _, err := io.Copy(buf, reader); err != nil {
				return err, true
			}
			hash := codec.Sha256(buf.Bytes())
			hashes = append(hashes, hash)
		}
		return nil, true
	})
	if err != nil {
		return "", err
	}

	if len(hashes) <= 0 {
		return "", utils.Error("no file found")
	}

	// 按哈希值排序以确保一致性
	sort.Strings(hashes)

	// 使用 | 连接所有哈希值，然后计算最终的哈希
	hash := codec.Sha256([]byte(strings.Join(hashes, "|")))

	// 缓存哈希值
	c.cachedHash = hash

	return hash, nil
}

// InvalidateHash 清除缓存的哈希值，在文件可能发生变化后调用
func (c *PreprocessingEmbed) InvalidateHash() {
	c.cachedHash = ""
}
