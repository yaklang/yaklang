package ssaapi

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
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

type ProcessFunc func(msg string, process float64)

type config struct {
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
	databasePath    string

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
	externLib               map[string]map[string]any
	externValue             map[string]any
	defineFunc              map[string]any
	externMethod            ssa.MethodBuilder
	externBuildValueHandler map[string]func(b *ssa.FunctionBuilder, id string, v any) (value ssa.Value)

	// other build options
	DatabaseProgramCacheHitter func(any)
	EnableCache                bool
	toProfile                  bool
	// for hash
	externInfo string
	// process ctx
	ctx context.Context
	// error
	err error
}

func defaultConfig(opts ...Option) (*config, error) {
	c := &config{
		language:                   "",
		SelectedLanguageBuilder:    nil,
		originEditor:               memedit.NewMemEditor(""),
		fs:                         filesys.NewLocalFs(),
		programPath:                ".",
		entryFile:                  make([]string, 0),
		externLib:                  make(map[string]map[string]any),
		externValue:                make(map[string]any),
		defineFunc:                 make(map[string]any),
		toProfile:                  false,
		DatabaseProgramCacheHitter: func(any) {},
	}

	for _, opt := range opts {
		opt(c)
	}
	if c.err != nil {
		return nil, c.err
	}
	return c, nil
}

func (c *config) CalcHash() string {
	return utils.CalcSha1(c.originEditor.GetSourceCode(), c.language, c.ignoreSyntaxErr, c.externInfo)
}

type Option func(*config)

func WithProcess(process ProcessFunc) Option {
	return func(c *config) {
		c.process = process
	}
}

func WithReCompile(b bool) Option {
	return func(c *config) {
		c.reCompile = b
	}
}

func WithStrictMode(b bool) Option {
	return func(c *config) {
		c.strictMode = b
	}
}

func WithError(err error) Option {
	return func(c *config) {
		c.err = err
	}
}

func WithLocalFs(path string) Option {
	return func(c *config) {
		c.fs = filesys.NewRelLocalFs(path)
		c.info = config_info{
			Kind:      "local",
			LocalFile: path,
		}.String()
	}
}
func WithFileSystem(fs fi.FileSystem) Option {
	return func(c *config) {
		if fs == nil {
			c.err = utils.Errorf("need set filesystem")
		} else {
			c.fs = fs
		}
	}
}

func WithConfigInfo(input map[string]any) Option {
	return func(c *config) {
		if input == nil {
			return
		}
		// json marshal info
		raw, err := json.Marshal(input)
		if err != nil {
			c.err = err
			return
		}
		info := string(raw)

		c.info = info
		if fs, err := initializeFromInfo(info); err != nil {
			c.err = err
		} else {
			c.fs = fs
		}
	}
}

func WithRawLanguage(input_language string) Option {
	if input_language == "" {
		return func(*config) {}
	}
	if language, err := consts.ValidateLanguage(input_language); err == nil {
		return WithLanguage(language)
	} else {
		return WithError(err)
	}
}

func WithLanguage(language consts.Language) Option {
	return func(c *config) {
		if language == "" {
			return
		}
		c.language = language
		if parser, ok := LanguageBuilders[language]; ok {
			c.SelectedLanguageBuilder = parser
		} else {
			log.Errorf("SSA not support language %s", language)
			c.SelectedLanguageBuilder = nil
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

func WithProgramDescription(desc string) Option {
	return func(c *config) {
		c.ProgramDescription = desc
	}
}

func WithDatabasePath(path string) Option {
	return func(c *config) {
		if utils.GetFirstExistedFile(path) == "" {
			return
		}
		if absPath, err := filepath.Abs(path); err != nil {
			log.Errorf("get abs path error: %v", err)
		} else {
			c.databasePath = absPath
		}
	}
}

// save to database, please set the program name
func WithProgramName(name string) Option {
	return func(c *config) {
		c.ProgramName = name
	}
}

func WithDatabaseProgramCacheHitter(h func(i any)) Option {
	return func(c *config) {
		c.DatabaseProgramCacheHitter = h
	}
}

func WithSaveToProfile(b ...bool) Option {
	return func(c *config) {
		if len(b) > 0 {
			c.toProfile = b[0]
		} else {
			c.toProfile = true
		}
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

func WithContext(ctx context.Context) Option {
	return func(c *config) {
		c.ctx = ctx
	}
}

func ParseProjectFromPath(path string, opts ...Option) (Programs, error) {
	if path != "" {
		opts = append(opts, WithLocalFs(path))
	}
	return ParseProject(opts...)
}

func ParseProjectWithFS(fs fi.FileSystem, opts ...Option) (Programs, error) {
	opts = append(opts, WithFileSystem(fs))
	return ParseProject(opts...)
}

func ParseProject(opts ...Option) (Programs, error) {
	config, err := defaultConfig(opts...)
	if err != nil {
		return nil, err
	}
	return config.parseProject()
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

// FromDatabase get program from database by program name
func FromDatabase(programName string) (*Program, error) {
	config, err := defaultConfig(WithProgramName(programName))
	if err != nil {
		return nil, err
	}
	return config.fromDatabase()
}

var Exports = map[string]any{
	"Parse":              Parse,
	"ParseLocalProject":  ParseProjectFromPath,
	"ParseProject":       ParseProject,
	"NewFromProgramName": FromDatabase,

	"withLanguage":      WithRawLanguage,
	"withConfigInfo":    WithConfigInfo,
	"withExternLib":     WithExternLib,
	"withExternValue":   WithExternValue,
	"withProgramName":   WithProgramName,
	"withDatabasePath":  WithDatabasePath,
	"withDescription":   WithProgramDescription,
	"withProcess":       WithProcess,
	"withEntryFile":     WithFileSystemEntry,
	"withReCompile":     WithReCompile,
	"withStrictMode":    WithStrictMode,
	"withSaveToProfile": WithSaveToProfile,
	"withContext":       WithContext,
	// "": with,
	// language:
	"Javascript": JS,
	"Yak":        Yak,
	"PHP":        PHP,
	"Java":       JAVA,
}
