package ssareducer

import (
	"embed"
	"io"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

type compileMethod func(string, io.Reader) ([]string, error)
type Config struct {
	entryFiles []string

	fs filesys.FileSystem

	compileMethod      compileMethod
	stopAtCompileError bool
}

func NewConfig(opt ...Option) *Config {
	c := &Config{
		entryFiles:         make([]string, 0),
		fs:                 nil,
		stopAtCompileError: false,
	}
	for _, o := range opt {
		o(c)
	}
	return c
}

type Option func(config *Config)

func WithFileSystem(fs filesys.FileSystem) Option {
	return func(config *Config) {
		config.fs = fs
	}
}

func WithEmbedFS(fs embed.FS) Option {
	return func(config *Config) {
		config.fs = filesys.NewEmbedFS(fs)
	}
}

func WithEntryFiles(filename ...string) Option {
	return func(config *Config) {
		config.entryFiles = append(config.entryFiles, filename...)
	}
}

func WithCompileMethod(handler compileMethod) Option {
	return func(config *Config) {
		config.compileMethod = handler
	}
}
