package ssaapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/gobwas/glob"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type ProcessFunc func(msg string, process float64)

type config struct {
	enableDatabase bool
	// program
	ProgramName        string
	ProgramDescription string

	// language
	language                consts.Language
	SelectedLanguageBuilder ssa.Builder
	LanguageBuilder         ssa.Builder

	// other compile options
	feedCode        bool
	ignoreSyntaxErr bool
	reCompile       bool
	strictMode      bool

	// input, code or project path
	originEditor *memedit.MemEditor
	// project
	info string
	// file system
	fs          fi.FileSystem
	entryFile   []string
	programPath string
	includePath []string

	// process
	process ProcessFunc

	// for build
	cacheTTL                []time.Duration
	externLib               map[string]map[string]any
	externValue             map[string]any
	defineFunc              map[string]any
	externMethod            ssa.MethodBuilder
	externBuildValueHandler map[string]func(b *ssa.FunctionBuilder, id string, v any) (value ssa.Value)

	// peephole
	peepholeSize int

	// other build options
	DatabaseProgramCacheHitter func(any)
	EnableCache                bool
	// for hash
	externInfo string
	// process ctx
	ctx context.Context

	excludeFile func(path, filename string) bool

	logLevel string
}

func defaultConfig(opts ...Option) (*config, error) {
	c := &config{
		language:                   "",
		SelectedLanguageBuilder:    nil,
		originEditor:               memedit.NewMemEditor(""),
		fs:                         filesys.NewLocalFs(),
		programPath:                ".",
		entryFile:                  make([]string, 0),
		cacheTTL:                   make([]time.Duration, 0),
		externLib:                  make(map[string]map[string]any),
		externValue:                make(map[string]any),
		defineFunc:                 make(map[string]any),
		DatabaseProgramCacheHitter: func(any) {},
		ctx:                        context.Background(),
		excludeFile: func(path, filename string) bool {
			return false
		},
		logLevel: "error",
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func DefaultExcludeFunc(patterns []string) (Option, error) {
	var compile []glob.Glob
	for _, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, err
		}
		compile = append(compile, g)
	}
	return func(c *config) error {
		c.excludeFile = func(dir string, path string) bool {
			for _, g := range compile {
				if match := g.Match(dir); match {
					return true
				}
				if match := g.Match(path); match {
					return true
				}
			}
			return false
		}
		return nil
	}, nil
}

func (c *config) CalcHash() string {
	return utils.CalcSha1(c.originEditor.GetSourceCode(), c.language, c.ignoreSyntaxErr, c.externInfo)
}

type Option func(*config) error

func (c *config) Processf(process float64, format string, arg ...any) {
	msg := fmt.Sprintf(format, arg...)
	if c.process != nil {
		c.process(msg, process)
	} else {
		log.Info(msg, process)
	}
}

func WithLogLevel(level string) Option {
	return func(c *config) error {
		log.SetLevel(level)
		c.logLevel = level
		return nil
	}
}

func WithCacheTTL(ttl time.Duration) Option {
	return func(c *config) error {
		c.cacheTTL = append(c.cacheTTL, ttl)
		return nil
	}
}

func WithExcludeFile(f func(path, filename string) bool) Option {
	return func(c *config) error {
		c.excludeFile = f
		return nil
	}
}
func WithProcess(process ProcessFunc) Option {
	return func(c *config) error {
		c.process = process
		return nil
	}
}

func WithReCompile(b bool) Option {
	return func(c *config) error {
		c.reCompile = b
		return nil
	}
}

func WithStrictMode(b bool) Option {
	return func(c *config) error {
		c.strictMode = b
		return nil
	}
}

func WithLocalFs(path string) Option {
	return func(c *config) error {
		WithConfigInfo(map[string]any{
			"kind":       "local",
			"local_file": path,
		})(c)
		return nil
	}
}

func WithFileSystem(fs fi.FileSystem) Option {
	return func(c *config) error {
		if fs == nil {
			return utils.Errorf("need set filesystem")
		}
		c.fs = getUnifiedSeparatorFs(fs)
		return nil
	}
}

func WithConfigInfoRaw(info string) Option {
	return func(c *config) error {
		c.info = info
		fs, err := c.parseFSFromInfo(info)
		if err != nil {
			return err
		}
		err = WithFileSystem(fs)(c)
		if err != nil {
			return err
		}
		return nil
	}
}
func WithConfigInfo(input map[string]any) Option {
	return func(c *config) error {
		if input == nil {
			return nil
		}
		// json marshal info
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		info := string(raw)

		return WithConfigInfoRaw(info)(c)
	}
}

func WithRawLanguage(input_language string) Option {
	if input_language == "" {
		return func(*config) error { return nil }
	}
	if language, err := consts.ValidateLanguage(input_language); err == nil {
		return WithLanguage(language)
	} else {
		return func(c *config) error {
			return err
		}
	}
}

func WithLanguage(language consts.Language) Option {
	return func(c *config) error {
		if language == "" {
			return nil
		}
		c.language = language
		if parser, ok := LanguageBuilders[language]; ok {
			c.SelectedLanguageBuilder = parser
		} else {
			log.Errorf("SSA not support language %s", language)
			c.SelectedLanguageBuilder = nil
		}
		return nil
	}
}

func WithFileSystemEntry(files ...string) Option {
	return func(c *config) error {
		c.entryFile = append(c.entryFile, files...)
		return nil
	}
}

func WithProgramPath(path string) Option {
	return func(c *config) error {
		c.programPath = path
		return nil
	}
}

func WithIncludePath(path ...string) Option {
	return func(c *config) error {
		c.includePath = append(c.includePath, path...)
		return nil
	}
}

func WithExternLib(name string, table map[string]any) Option {
	return func(c *config) error {
		c.externLib[name] = table
		return nil
	}
}

func WithExternValue(table map[string]any) Option {
	return func(c *config) error {
		for name, value := range table {
			c.externValue[name] = value
		}
		return nil
	}
}

func WithExternMethod(b ssa.MethodBuilder) Option {
	return func(c *config) error {
		c.externMethod = b
		return nil
	}
}

func WithExternBuildValueHandler(id string, callback func(b *ssa.FunctionBuilder, id string, v any) ssa.Value) Option {
	return func(c *config) error {
		if c.externBuildValueHandler == nil {
			c.externBuildValueHandler = make(map[string]func(b *ssa.FunctionBuilder, id string, v any) ssa.Value)
		}
		c.externBuildValueHandler[id] = callback
		return nil
	}
}

func WithPeepholeSize(size int) Option {
	return func(c *config) error {
		c.peepholeSize = size
		return nil
	}
}

func WithIgnoreSyntaxError(b ...bool) Option {
	return func(c *config) error {
		if len(b) > 0 {
			c.ignoreSyntaxErr = b[0]
		} else {
			c.ignoreSyntaxErr = true
		}
		return nil
	}
}

func WithExternInfo(info string) Option {
	return func(c *config) error {
		c.externInfo = info
		return nil
	}
}

func WithDefineFunc(table map[string]any) Option {
	return func(c *config) error {
		for name, t := range table {
			c.defineFunc[name] = t
		}
		return nil
	}
}

func WithFeedCode(b ...bool) Option {
	return func(c *config) error {
		if len(b) > 0 {
			c.feedCode = b[0]
		} else {
			c.feedCode = true
		}
		return nil
	}
}

func WithProgramDescription(desc string) Option {
	return func(c *config) error {
		c.ProgramDescription = desc
		return nil
	}
}

// save to database, please set the program name
func WithProgramName(name string) Option {
	return func(c *config) error {
		c.ProgramName = name
		c.enableDatabase = true
		return nil
	}
}

func WithDatabaseProgramCacheHitter(h func(i any)) Option {
	return func(c *config) error {
		c.DatabaseProgramCacheHitter = h
		return nil
	}
}

func WithEnableCache(b ...bool) Option {
	return func(c *config) error {
		if len(b) > 0 {
			c.EnableCache = b[0]
		} else {
			c.EnableCache = true
		}
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *config) error {
		c.ctx = ctx
		return nil
	}
}

func getUnifiedSeparatorFs(fs fi.FileSystem) fi.FileSystem {
	return filesys.NewUnifiedFS(fs,
		filesys.WithUnifiedFsSeparator(ssadb.IrSourceFsSeparators),
	)
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
	config, err := defaultConfig(opts...)
	if err != nil {
		return nil, err
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
	if p.config == nil || !p.config.feedCode || p.config.SelectedLanguageBuilder == nil {
		return utils.Errorf("not support language %s", p.config.language)
	}

	raw, err := io.ReadAll(code)
	if err != nil {
		return err
	}

	return p.config.feed(p.Program, memedit.NewMemEditor(string(raw)))
}

/*
YaklangScriptChecking is a function that checks the syntax of a Yaklang script.

Input: code string, pluginType: `"yak" "mitm" "port-scan" "codec" "syntaxflow"`

Return: []*result.StaticAnalyzeResult
*/
func YaklangScriptChecking(code string, pluginType string) []any {
	log.Warn("YaklangScriptChecking is not implemented! Please contact developers to fix it.")
	return nil
}

func RegisterExport(name string, value any) {
	if _, ok := Exports[name]; !ok {
		log.Warnf("ssa Export [%s] create by Register but no default implement", name)
	}
	Exports[name] = value
}

var Exports = map[string]any{
	"Parse":              Parse,
	"ParseLocalProject":  ParseProjectFromPath,
	"ParseProject":       ParseProject,
	"NewFromProgramName": FromDatabase,
	"NewProgramFromDB":   FromDatabase,

	"withLanguage":           WithRawLanguage,
	"withConfigInfo":         WithConfigInfo,
	"withExternLib":          WithExternLib,
	"withExternValue":        WithExternValue,
	"withProgramName":        WithProgramName,
	"withDescription":        WithProgramDescription,
	"withProcess":            WithProcess,
	"withEntryFile":          WithFileSystemEntry,
	"withReCompile":          WithReCompile,
	"withStrictMode":         WithStrictMode,
	"withContext":            WithContext,
	"withPeepholeSize":       WithPeepholeSize,
	"withExcludeFile":        WithExcludeFile,
	"withDefaultExcludeFunc": DefaultExcludeFunc,

	// language:
	"Javascript": JS,
	"Yak":        Yak,
	"PHP":        PHP,
	"Java":       JAVA,

	/// static analyze
	"YaklangScriptChecking": YaklangScriptChecking,

	// result
	"NewResultFromDB": LoadResultByID,
}
