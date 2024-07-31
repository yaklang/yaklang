package ssaapi

import (
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type config struct {
	language        consts.Language
	LanguageBuilder ssa.Builder
	feedCode        bool
	ignoreSyntaxErr bool

	// input, code or project path
	originEditor *memedit.MemEditor
	// project
	fs          fi.FileSystem
	entryFile   []string
	programPath string
	includePath []string

	externLib               map[string]map[string]any
	externValue             map[string]any
	defineFunc              map[string]any
	externMethod            ssa.MethodBuilder
	externBuildValueHandler map[string]func(b *ssa.FunctionBuilder, id string, v any) (value ssa.Value)

	DatabaseProgramName        string
	DatabaseProgramCacheHitter func(any)
	EnableCache                bool
	// for hash
	externInfo string
}

func defaultConfig() *config {
	return &config{
		language:                   "",
		LanguageBuilder:            nil,
		originEditor:               memedit.NewMemEditor(""),
		fs:                         filesys.NewLocalFs(),
		programPath:                ".",
		entryFile:                  make([]string, 0),
		externLib:                  make(map[string]map[string]any),
		externValue:                make(map[string]any),
		defineFunc:                 make(map[string]any),
		DatabaseProgramCacheHitter: func(any) {},
	}
}

func (c *config) CalcHash() string {
	return utils.CalcSha1(c.originEditor.GetSourceCode(), c.language, c.ignoreSyntaxErr, c.externInfo)
}

type Option func(*config)

func WithLanguage(language consts.Language) Option {
	return func(c *config) {
		c.language = language
		if parser, ok := LanguageBuilders[language]; ok {
			c.LanguageBuilder = parser
		} else {
			log.Errorf("SSA not support language %s", language)
			c.LanguageBuilder = nil
		}
	}
}

func WithFileSystemEntry(files ...string) Option {
	return func(c *config) {
		c.entryFile = append(c.entryFile, files...)
	}
}

func WithProgramPath(path string) Option {
	return func(c *config) {
		c.programPath = path
	}
}

func WithIncludePath(path ...string) Option {
	return func(c *config) {
		c.includePath = append(c.includePath, path...)
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

func WithExternBuildValueHandler(id string, callback func(b *ssa.FunctionBuilder, id string, v any) ssa.Value) Option {
	return func(c *config) {
		if c.externBuildValueHandler == nil {
			c.externBuildValueHandler = make(map[string]func(b *ssa.FunctionBuilder, id string, v any) ssa.Value)
		}
		c.externBuildValueHandler[id] = callback
	}
}

func WithIgnoreSyntaxError(b ...bool) Option {
	return func(c *config) {
		if len(b) > 0 {
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
		if len(b) > 0 {
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

func WithEnableCache(b ...bool) Option {
	return func(c *config) {
		if len(b) > 0 {
			c.EnableCache = b[0]
		} else {
			c.EnableCache = true
		}
	}
}

func ParseProjectFromPath(path string, opts ...Option) (Programs, error) {
	opts = append(opts, WithProgramPath(path))
	return ParseProject(filesys.NewLocalFs(), opts...)
}

func ParseProject(fs fi.FileSystem, opts ...Option) (Programs, error) {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	config.fs = fs
	if config.fs == nil {
		return nil, utils.Errorf("need set filesystem")
	}
	ret, err := config.parseProject()
	return ret, err
}

var ttlSSAParseCache = createCache(10 * time.Second)

func createCache(ttl time.Duration) *utils.CacheWithKey[string, *Program] {
	cache := utils.NewTTLCacheWithKey[string, *Program](ttl)
	return cache
}

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
	if input != nil {
		raw, err := io.ReadAll(input)
		if err != nil {
			log.Warnf("read input error: %v", err)
		}
		config.originEditor = memedit.NewMemEditor(string(raw))
	}

	hash := config.CalcHash()
	if config.EnableCache {
		if prog, ok := ttlSSAParseCache.Get(hash); ok {
			return prog, nil
		}
	}

	ret, err := config.parseFile()
	if err == nil && config.EnableCache {
		ttlSSAParseCache.SetWithTTL(hash, ret, 30*time.Minute)
	}
	return ret, err
}

func (p *Program) Feed(code io.Reader) error {
	if p.config == nil || !p.config.feedCode || p.config.LanguageBuilder == nil {
		return utils.Errorf("not support language %s", p.config.language)
	}

	raw, err := io.ReadAll(code)
	if err != nil {
		return err
	}

	return p.config.feed(p.Program, memedit.NewMemEditor(string(raw)))
}

func FromDatabase(programName string, opts ...Option) (*Program, error) {
	config := defaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	config.DatabaseProgramName = programName

	return config.fromDatabase()
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
