package ssareducer

import (
	"context"
	"embed"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

var log = ssalog.Log

type (
	compileMethod func(string, string) ([]string, error)
	Config        struct {
		entryFiles []string

		ProgramName string

		fs fi.FileSystem

		compileMethod      compileMethod
		stopAtCompileError bool

		ctx context.Context
	}
)

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

func WithProgramName(name string) Option {
	return func(config *Config) {
		config.ProgramName = name
	}
}

func WithStrictMode(b bool) Option {
	return func(config *Config) {
		config.stopAtCompileError = b
	}
}

func WithFileSystem(fs fi.FileSystem) Option {
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

func WithContext(ctx context.Context) Option {
	return func(config *Config) {
		config.ctx = ctx
	}
}

func (c *Config) GetCancelFunc() context.CancelFunc {
	var cancelFunc context.CancelFunc
	if c.ctx == nil {
		c.ctx, cancelFunc = context.WithCancel(context.Background())
	} else {
		c.ctx, cancelFunc = context.WithCancel(c.ctx)
	}
	return cancelFunc
}
