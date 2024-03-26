package ssareducer

import "embed"

type Config struct {
	// suffix
	exts []string

	// embed fs for test generally
	embedFS *embed.FS

	compileMethods     func(*ReducerCompiler, string) ([]string, error)
	stopAtCompileError bool
}

func NewConfig() *Config {
	c := &Config{}
	return c
}

type Option func(config *Config)

func WithFileExt(ext string, others ...string) Option {
	return func(config *Config) {
		config.exts = append(config.exts, ext)
		config.exts = append(config.exts, others...)
	}
}

func WithEmbedFS(fs embed.FS) Option {
	return func(config *Config) {
		config.embedFS = &fs
	}
}

func WithCompileMethod(handler func(*ReducerCompiler, string) ([]string, error)) Option {
	return func(config *Config) {
		config.compileMethods = handler
	}
}
