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
	Parse(string, bool, func(*ssa.FunctionBuilder)) (*ssa.Program, error)
	Feed(string, bool, *ssa.Program)
}

var (
	LanguageParsers = map[Language]LanguageParser{
		Yak: yak2ssa.NewParser(),
		JS:  js2ssa.NewParser(),
	}
)

type config struct {
	language        Language
	Parser          LanguageParser
	code            string
	feedCode        bool
	ignoreSyntaxErr bool

	externLib    map[string]map[string]any
	externValue  map[string]any
	defineFunc   map[string]any
	externMethod ssa.MethodBuilder
	// for hash
	externInfo string
}

func defaultConfig(code string) *config {
	return &config{
		language:    Yak,
		Parser:      LanguageParsers[Yak],
		code:        code,
		externLib:   make(map[string]map[string]any),
		externValue: make(map[string]any),
		defineFunc:  make(map[string]any),
	}
}

func (c *config) CaclHash() string {
	return utils.CalcSha1(c.code, c.language, c.ignoreSyntaxErr, c.externInfo)
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

func WithExternInfo(info string) Option {
	return func(c *config) {
		c.externInfo = info
	}
}

func WithDefineFunc(table map[string]any) Option {
	return func(c *config) {
		for name, t := range table {
			c.defineFunc[name] = t
		}
	}
}

func WithFeedCode(b ...bool) Option {
	return func(c *config) {
		if len(b) > 1 {
			c.feedCode = b[0]
		} else {
			c.feedCode = true
		}
	}
}

var ttlSSAParseCache = ttlcache.NewCache()

func Parse(code string, opts ...Option) (*Program, error) {
	config := defaultConfig(code)
	for _, opt := range opts {
		opt(config)
	}
	if config.Parser == nil {
		return nil, utils.Errorf("not support language %s", config.language)
	}
	var ret *Program
	hash := config.CaclHash()
	if prog, ok := ttlSSAParseCache.Get(hash); ok {
		ret = prog.(*Program)
	} else {
		prog, err := parseWithConfig(config)
		if err != nil {
			return nil, utils.Wrapf(err, "parse error")
		}
		ret = NewProgram(prog)
		ret.AddConfig(config)
	}
	ttlSSAParseCache.SetWithTTL(hash, ret, 30*time.Minute)
	return ret, nil
}

func parseWithConfig(c *config) (*ssa.Program, error) {
	callback := func(fb *ssa.FunctionBuilder) {
		fb.WithExternLib(c.externLib)
		fb.WithExternValue(c.externValue)
		fb.WithExternMethod(c.externMethod)
		fb.WithDefineFunction(c.defineFunc)
	}
	return c.Parser.Parse(c.code, c.ignoreSyntaxErr, callback)
}

func (p *Program) Feed(code string) {
	if p.config == nil || !p.config.feedCode || p.config.Parser == nil {
		return
	}
	p.config.Parser.Feed(code, p.config.ignoreSyntaxErr, p.Program)
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
