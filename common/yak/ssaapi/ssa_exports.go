package ssaapi

import (
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/utils"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

type Language string

const (
	JS  Language = "js"
	Yak Language = "yak"
)

type LanguageParser interface {
	Parse(string, bool, func(*ssa.FunctionBuilder)) *ssa.Program
}

var (
	LanguageParsers = map[Language]LanguageParser{
		Yak: yak2ssa.NewParser(),
		JS:  js2ssa.NewParser(),
	}
)

type config struct {
	language Language
	Parser   LanguageParser
	// code     string
	ignoreSyntaxErr bool

	externLib   map[string]map[string]any
	externValue map[string]any
	// externType  map[string]any
	externMethod ssa.MethodBuilder
}

func defaultConfig() *config {
	return &config{
		language: Yak,
		Parser:   LanguageParsers[Yak],
		// code:        "",
		externLib:   make(map[string]map[string]any),
		externValue: make(map[string]any),
	}
}

type Option func(*config)

func WithLanguage(language Language) Option {
	return func(c *config) {
		c.language = language
		if parser, ok := LanguageParsers[language]; ok {
			c.Parser = parser
		}
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

func WithIgnoreSyntaxError(b ...bool) Option {
	return func(c *config) {
		if len(b) > 1 {
			c.ignoreSyntaxErr = b[0]
		} else {
			c.ignoreSyntaxErr = true
		}
	}

}

func Parse(code string, opts ...Option) *Program {
	return parse(code, opts...)
}

var ttlSSAParseCache = ttlcache.NewCache()

func parse(code string, opts ...Option) *Program {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	if config.Parser == nil {
		return nil
	}
	hash := utils.CalcSha1(code, config.language)
	if prog, ok := ttlSSAParseCache.Get(hash); ok {
		ttlSSAParseCache.SetWithTTL(hash, prog, 30*time.Minute) // refresh
		return prog.(*Program)
	}

	prog := NewProgram(parseWithConfig(code, config))

	ttlSSAParseCache.SetWithTTL(hash, prog, 30*time.Minute)
	return prog
}

func parseWithConfig(code string, c *config) *ssa.Program {
	callback := func(fb *ssa.FunctionBuilder) {
		fb.WithExternLib(c.externLib)
		fb.WithExternValue(c.externValue)
		fb.WithExternMethod(c.externMethod)
	}
	return c.Parser.Parse(code, c.ignoreSyntaxErr, callback)
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
