package filesys

import (
	"embed"
	"github.com/gobwas/glob"
	"github.com/kr/fs"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"
)

type dirChain struct {
	dirGlob string
	globIns glob.Glob
	opts    []Option
}

type exactChain struct {
	dirpath string
	opts    []Option
}

func NewConfig() *Config {
	return &Config{
		onStart: func(base string, isDir bool) error {
			return nil
		},
		onStat: func(isDir bool, pathname string, info os.FileInfo) error {
			return nil
		},
		onDirStat: func(pathname string, info os.FileInfo) error {
			return nil
		},
		onFileStat: func(pathname string, info os.FileInfo) error {
			return nil
		},
		chains:        nil,
		noStopWhenErr: false,
		fileLimit:     100000,
		dirLimit:      100000,
		totalLimit:    100000,
		fileSystem:    &localFs{},
	}
}

type Config struct {
	onStart    func(base string, isDir bool) error
	onStat     func(isDir bool, pathname string, info os.FileInfo) error
	onDirStat  func(pathname string, info os.FileInfo) error
	onFileStat func(pathname string, info os.FileInfo) error

	chains []*dirChain

	noStopWhenErr bool

	fileLimit  int64
	dirLimit   int64
	totalLimit int64

	fileSystem fs.FileSystem
}

type Option func(*Config)

func WithStat(f func(isDir bool, pathname string, info os.FileInfo) error) Option {
	return func(c *Config) {
		c.onStat = f
	}
}

func WithDirStat(f func(pathname string, info os.FileInfo) error) Option {
	return func(c *Config) {
		c.onDirStat = f
	}
}

func WithFileStat(f func(pathname string, info os.FileInfo) error) Option {
	return func(c *Config) {
		c.onFileStat = f
	}
}

func WithOnStart(f func(basename string, isDir bool) error) Option {
	return func(c *Config) {
		c.onStart = f
	}
}

func WithDirMatches(raw any, opts ...Option) Option {
	return func(config *Config) {
		dirs := utils.InterfaceToStringSlice(raw)
		dirs = funk.ReverseStrings(dirs)
		var opt Option
		for _, dir := range dirs {
			if opt == nil {
				opt = WithDirMatch(dir, opts...)
			} else {
				opt = WithDirMatch(dir, opt)
			}
		}
		if opt != nil {
			opt(config)
		}
	}
}

func WithDirMatch(globDir string, opts ...Option) Option {
	return func(c *Config) {
		ins, err := glob.Compile(globDir, '/')
		if err != nil {
			log.Errorf("glob-dir: %v compile failed: %s", globDir, err.Error())
			return
		}
		c.chains = append(c.chains, &dirChain{
			dirGlob: globDir,
			opts:    opts,
			globIns: ins,
		})
	}
}

func WithFileSystem(f fs.FileSystem) Option {
	return func(config *Config) {
		config.fileSystem = f
	}
}

func WithEmbedFS(f embed.FS) Option {
	return func(config *Config) {
		config.fileSystem = fromEmbedFS(f)
	}
}

type embedFs struct {
	f embed.FS
}

func (e embedFs) ReadDir(dirname string) ([]os.FileInfo, error) {
	ns, err := e.f.ReadDir(dirname)
	if err != nil {
		return nil, err
	}
	var infos = make([]os.FileInfo, 0, len(ns))
	for _, n := range ns {
		info, err := n.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

/*
// A FileInfo describes a file and is returned by Stat.
type FileInfo interface {
	Name() string       // base name of the file
	Size() int64        // length in bytes for regular files; system-dependent for others
	Mode() FileMode     // file mode bits
	ModTime() time.Time // modification time
	IsDir() bool        // abbreviation for Mode().IsDir()
	Sys() any           // underlying data source (can return nil)
}
*/

type embedDirInfo string

func (e embedDirInfo) Name() string {
	_, n := path.Split(string(e))
	return n
}

func (e embedDirInfo) Size() int64        { return 0 }
func (e embedDirInfo) Mode() os.FileMode  { return os.ModeDir }
func (e embedDirInfo) ModTime() time.Time { return time.Time{} }
func (e embedDirInfo) IsDir() bool        { return true }
func (e embedDirInfo) Sys() interface{}   { return nil }

func (e embedFs) Lstat(name string) (os.FileInfo, error) {
	f, err := e.f.Open(name)
	if err != nil {
		//_, err := e.f.ReadDir(name)
		//if err != nil {
		//	return nil, err
		//}
		//var i os.FileInfo = embedDirInfo(name)
		//return i, nil
		return nil, err
	}
	return f.Stat()
}

func (e embedFs) Join(elem ...string) string {
	return path.Join(elem...)
}

func fromEmbedFS(fs2 embed.FS) fs.FileSystem {
	return &embedFs{fs2}
}

// local filesystem
type localFs struct{}

func (f *localFs) ReadDir(dirname string) ([]os.FileInfo, error) { return ioutil.ReadDir(dirname) }

func (f *localFs) Lstat(name string) (os.FileInfo, error) { return os.Lstat(name) }

func (f *localFs) Join(elem ...string) string { return filepath.Join(elem...) }
