package ssaapi

import (
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/utils"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

var ttlSSAParseCache = ttlcache.NewCache()

type config struct {
	language Language
	code     string

	externLib   map[string]map[string]any
	externValue map[string]any
	// externType  map[string]any
	externMethod ssa.MethodBuilder
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
		// c.externValue = table
		for name, value := range table {
			// this value set again
			// if _, ok := c.externValue[name]; !ok {
			// 	// skip
			// }
			c.externValue[name] = value
		}
	}
}

func WithExternMethod(b ssa.MethodBuilder) Option {
	return func(c *config) {
		c.externMethod = b
	}
}

func Parse(code string, opts ...Option) *Program {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	hash := utils.CalcSha1(code, config.language)
	if prog, ok := ttlSSAParseCache.Get(hash); ok {
		return prog.(*Program)
	}

	callback := func(fb *ssa.FunctionBuilder) {
		fb.WithExternLib(config.externLib)
		fb.WithExternValue(config.externValue)
		fb.WithExternMethod(config.externMethod)
	}

	var ret *ssa.Program
	switch config.language {
	case JS:
		ret = js2ssa.ParseSSA(code, callback)
	case Yak:
		ret = yak2ssa.ParseSSA(code, callback)
	}
	prog := NewProgram(ret)
	ttlSSAParseCache.SetWithTTL(hash, prog, 30*time.Minute)
	return prog
}

var Exports = map[string]any{
	"Parse": Parse,

	"withLanguage":    WithLanguage,
	"withExternLib":   WithExternLib,
	"withExternValue": WithExternValue,
	// language:
	"Javascript": JS,
	"Yak":        Yak,
}
