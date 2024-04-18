package ssaapi

import (
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type config struct {
	language        Language
	Builder         Builder
	feedCode        bool
	ignoreSyntaxErr bool

	// input, code or project path
	code io.Reader
	fs   filesys.FileSystem

	externLib    map[string]map[string]any
	externValue  map[string]any
	defineFunc   map[string]any
	externMethod ssa.MethodBuilder

	DatabaseProgramName        string
	DatabaseProgramCacheHitter func(any)
	// for hash
	externInfo string
}

func defaultConfig() *config {
	return &config{
		language:    Yak,
		Builder:     LanguageBuilders[Yak],
		code:        nil,
		fs:          nil,
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
			c.Builder = parser
		} else {
			c.Builder = nil
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
func WithDatabaseProgramName(name string) Option {
	return func(c *config) {
		c.DatabaseProgramName = name
	}
}

func WithDatabaseProgramCacheHitter(h func(i any)) Option {
	return func(c *config) {
		c.DatabaseProgramCacheHitter = h
	}
}

func ParseProjectFromPath(path string, opts ...Option) (*Program, error) {
	fs := filesys.NewLocalFs(path)
	return ParseProject(fs, opts...)
}

func ParseProject(fs filesys.FileSystem, opts ...Option) (*Program, error) {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	config.fs = fs
	if config.Builder == nil {
		return nil, utils.Errorf("not support language %s", config.language)
	}
	var ret *Program
	prog, err := config.parse()
	if err != nil {
		return nil, utils.Wrapf(err, "parse error")
	}
	ret = NewProgram(prog)
	ret.AddConfig(config)
	return ret, nil
}

var ttlSSAParseCache = utils.NewTTLCache[*Program](30 * time.Minute)

func ClearCache() {
	ttlSSAParseCache.Purge()
}

// Parse parse code to ssa.Program
func Parse(code string, opts ...Option) (*Program, error) {
	input := strings.NewReader(code)
	return ParseFromReader(input, opts...)
}

// ParseFromReader parse simple file to ssa.Program
func ParseFromReader(input io.Reader, opts ...Option) (*Program, error) {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	if config.Builder == nil {
		return nil, utils.Errorf("not support language %s", config.language)
	}
	config.code = input
	var ret *Program

	hash := config.CaclHash()
	if prog, ok := ttlSSAParseCache.Get(hash); ok {
		ret = prog
	} else {
		prog, err := config.parse()
		if err != nil {
			return nil, utils.Wrapf(err, "parse error")
		}
		ret = NewProgram(prog)
		ret.AddConfig(config)
	}
	ttlSSAParseCache.SetWithTTL(hash, ret, 30*time.Minute)
	return ret, nil
}

func (p *Program) Feed(code io.Reader) error {
	if p.config == nil || !p.config.feedCode || p.config.Builder == nil {
		return utils.Errorf("not support language %s", p.config.language)
	}
	return p.config.feed(p.Program, code)
}

var Exports = map[string]any{
	"Parse": Parse,

	"withLanguage":            WithLanguage,
	"withExternLib":           WithExternLib,
	"withExternValue":         WithExternValue,
	"withDatabaseProgramName": WithDatabaseProgramName,
	// language:
	"Javascript": JS,
	"Yak":        Yak,
	"PHP":        PHP,
	"Java":       JAVA,
}
