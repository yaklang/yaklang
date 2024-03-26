package filesys

import (
	"embed"
	"github.com/gobwas/glob"
	"github.com/kr/fs"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
)

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

func WithDir(i any, opts ...Option) Option {
	switch i.(type) {
	case []byte, string, []rune:
		return WithDirMatch(utils.InterfaceToString(i), opts...)
	default:
		return WithDirMatches(utils.InterfaceToStringSlice(i), opts...)
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
	return WithFileStat(func(pathname string, info os.FileInfo) (err error) {
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
	return WithDirStat(func(pathname string, info os.FileInfo) (err error) {
		defer func() {
			if e := recover(); e != nil {
				err = utils.Errorf("onDirStat failed: %v", e)
			}
		}()
		h(pathname, info)
		return nil
	})
}
