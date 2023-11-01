package yak

import (
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

type config struct {
	language Language
	code     string

	externLib   map[string]map[string]any
	externValue map[string]any
	// externType  map[string]any
	// externMethod
}

func defaultConfig() *config {
	return &config{
		language:    Yak,
		code:        "",
		externLib:   make(map[string]map[string]any),
		externValue: make(map[string]any),
	}
}

type Option func(*config)

type Language string

const (
	JS  Language = "js"
	Yak Language = "yak"
)

func WithLanguage(language Language) Option {
	return func(c *config) {
		c.language = language
	}
}

func WithExternLib(name string, table map[string]any) Option {
	return func(c *config) {
		c.externLib[name] = table
	}
}

func WithExternValue(table map[string]any) Option {
	return func(c *config) {
		c.externValue = table
	}
}
func Parse(code string, opts ...Option) *ssa.Program {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	callback := func(fb *ssa.FunctionBuilder) {
		fb.WithExternLib(config.externLib)
		fb.WithExternValue(config.externValue)
	}

	var ret *ssa.Program
	switch config.language {
	case JS:
		ret = js2ssa.ParseSSA(code, callback)
	case Yak:
		ret = yak2ssa.ParseSSA(code, callback)
	}
	return ret
}

var SSAExports = map[string]any{
	"Parse": Parse,

	"withLanguage": WithLanguage,
	// language:
	"Javascript": JS,
	"Yak":        Yak,
}
