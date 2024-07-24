package filesys

import (
	"embed"
	"io/fs"
	"os"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type dirResult struct {
	dir  string
	opts []Option
}

type dirMatch struct {
	inst glob.Glob
	opts []Option
}

type (
	FileStat func(string, fs.FileInfo) error
	DirStat  func(string, fs.FileInfo) error
	Config   struct {
		onStart      func(base string, isDir bool) error
		onStat       func(isDir bool, pathname string, info os.FileInfo) error
		onDirStat    DirStat
		onFileStat   FileStat
		onDirWalkEnd func(string) error

		noStopWhenErr bool

		RecursiveDirectory bool

		fileLimit  int64
		dirLimit   int64
		totalLimit int64

		fileSystem fi.FileSystem

		dirMatch []*dirMatch
	}
)

func NewConfig() *Config {
	return &Config{
		noStopWhenErr:      false,
		RecursiveDirectory: true,
		fileLimit:          100000,
		dirLimit:           100000,
		totalLimit:         100000,
		fileSystem:         NewLocalFs(),
	}
}

type Option func(*Config)

func WithStat(f func(isDir bool, pathname string, info os.FileInfo) error) Option {
	return func(c *Config) {
		c.onStat = f
	}
}

func WithRecursiveDirectory(b bool) Option {
	return func(c *Config) {
		c.RecursiveDirectory = b
	}
}

func WithDirStat(f DirStat) Option {
	return func(c *Config) {
		c.onDirStat = f
	}
}

func WithFileStat(f FileStat) Option {
	return func(c *Config) {
		c.onFileStat = f
	}
}

func WithOnStart(f func(basename string, isDir bool) error) Option {
	return func(c *Config) {
		c.onStart = f
	}
}

func WithDir(globDir string, opts ...Option) Option {
	return func(c *Config) {
		if c.fileSystem == nil {
			log.Errorf("file system is nil")
			return
		}

		// if the separator is not the same as the file system, replace it
		for _, separator := range []rune{'/', '\\'} {
			if c.fileSystem.GetSeparators() == separator {
				continue
			}
			if !strings.Contains(globDir, string(separator)) {
				strings.ReplaceAll(globDir, string(separator), string(c.fileSystem.GetSeparators()))
			}
		}

		ins, err := glob.Compile(globDir, c.fileSystem.GetSeparators())
		if err != nil {
			log.Errorf("glob-dir: %v compile failed: %s", globDir, err.Error())
			return
		}
		// log.Infof("dir match: %v: inst: %v", globDir, ins)
		c.dirMatch = append(c.dirMatch, &dirMatch{
			// dir:  globDir,
			inst: ins,
			opts: opts,
		})
	}
}

func WithFileSystem(f fi.FileSystem) Option {
	return func(config *Config) {
		config.fileSystem = f
	}
}

func WithEmbedFS(f embed.FS) Option {
	return func(config *Config) {
		config.fileSystem = NewEmbedFS(f)
	}
}

func WithDirWalkEnd(handle func(path string) error) Option {
	return func(config *Config) {
		config.onDirWalkEnd = handle
	}
}

// onReady will be called when the walker is ready to start walking.
func withYaklangOnStart(h func(name string, isDir bool)) Option {
	return WithOnStart(func(basename string, isDir bool) (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Errorf("onStart failed: %v", e)
			}
		}()
		h(basename, isDir)
		return nil
	})
}

// onStat will be called when the walker met one file description.
func withYaklangStat(h func(isDir bool, pathname string, info os.FileInfo)) Option {
	return WithStat(func(isDir bool, pathname string, info os.FileInfo) (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Errorf("onStat failed: %v", e)
			}
		}()
		h(isDir, pathname, info)
		return nil
	})
}

// onFileStat will be called when the walker met one file.
func withYaklangFileStat(h func(pathname string, info os.FileInfo)) Option {
	return WithFileStat(func(pathname string, info fs.FileInfo) (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Errorf("onFileStat failed: %v", e)
			}
		}()
		h(pathname, info)
		return nil
	})
}

// onDirStat will be called when the walker met one directory.
func withYaklangDirStat(h func(pathname string, info os.FileInfo)) Option {
	return WithDirStat(func(pathname string, info fs.FileInfo) (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Errorf("onDirStat failed: %v", e)
			}
		}()
		h(pathname, info)
		return nil
	})
}
