package ssaapi

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type config struct {
	language        Language
	Build           Build
	code            string
	feedCode        bool
	ignoreSyntaxErr bool

	externLib    map[string]map[string]any
	externValue  map[string]any
	defineFunc   map[string]any
	externMethod ssa.MethodBuilder

	DataBaseProgramName string
	// for hash
	externInfo string
}

func defaultConfig(code string) *config {
	return &config{
		language:    Yak,
		Build:       LanguageBuilders[Yak],
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
		if parser, ok := LanguageBuilders[language]; ok {
			c.Build = parser
		} else {
			c.Build = nil
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

// save to database, please set the program name
func WithDataBase(name string) Option {
	return func(c *config) {
		c.DataBaseProgramName = name
	}
}

var ttlSSAParseCache = utils.NewTTLCache[*Program](30 * time.Minute)

func Parse(code string, opts ...Option) (*Program, error) {
	config := defaultConfig(code)
	for _, opt := range opts {
		opt(config)
	}
	if config.Build == nil {
		return nil, utils.Errorf("not support language %s", config.language)
	}
	var ret *Program

	hash := config.CaclHash()
	if prog, ok := ttlSSAParseCache.Get(hash); ok {
		ret = prog
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
	return parse(c, nil)
}

func (p *Program) Feed(code string) {
	if p.config == nil || !p.config.feedCode || p.config.Build == nil {
		return
	}
	feed(p.config, p.Program, code)
}

var Exports = map[string]any{
	"Parse": Parse,

	"withLanguage":    WithLanguage,
	"withExternLib":   WithExternLib,
	"withExternValue": WithExternValue,
	"withDataBase":    WithDataBase,
	// language:
	"Javascript": JS,
	"Yak":        Yak,
	"PHP":        PHP,
	"Java":       JAVA,
}
