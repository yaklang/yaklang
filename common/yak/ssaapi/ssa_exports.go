package ssaapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssaproject"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"

	"github.com/gobwas/glob"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type ProcessFunc func(msg string, process float64)

type Config struct {
	databaseKind   ssa.ProgramCacheKind
	programSaveTTL time.Duration
	// project
	ProjectName string
	// program
	ProgramName        string
	ProgramDescription string

	// language
	language        consts.Language
	LanguageBuilder ssa.Builder

	// other compile options
	feedCode        bool
	ignoreSyntaxErr bool

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

	// other build options
	DatabaseProgramCacheHitter func(any)
	EnableCache                bool
	// for hash
	externInfo string
	// process ctx
	ctx context.Context

	excludeFile func(path, filename string) bool

	logLevel string

	astSequence ssareducer.ASTSequenceType

	*ssaconfig.Config
}

func DefaultConfig(opts ...Option) (*Config, error) {
	sc, _ := ssaconfig.New(ssaconfig.ModeSSACompile)
	c := &Config{
		language:                   "",
		LanguageBuilder:            nil,
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
		logLevel:    "error",
		astSequence: ssareducer.OutOfOrder,
		Config:      sc,
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
	return func(c *Config) error {
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

func (c *Config) CalcHash() string {
	return utils.CalcSha1(c.originEditor.GetSourceCode(), c.language, c.ignoreSyntaxErr, c.externInfo)
}

type Option func(*Config) error

func (c *Config) Processf(process float64, format string, arg ...any) {
	msg := fmt.Sprintf(format, arg...)
	if c.process != nil {
		c.process(msg, process)
	} else {
		log.Info(msg, process)
	}
}

func WithASTOrder(sequence ssareducer.ASTSequenceType) Option {
	return func(c *Config) error {
		c.astSequence = sequence
		return nil
	}
}

func WithLogLevel(level string) Option {
	return func(c *Config) error {
		log.SetLevel(level)
		c.logLevel = level
		return nil
	}
}

func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Config) error {
		c.cacheTTL = append(c.cacheTTL, ttl)
		return nil
	}
}

func WithExcludeFile(f func(path, filename string) bool) Option {
	return func(c *Config) error {
		c.excludeFile = f
		return nil
	}
}
func WithProcess(process ProcessFunc) Option {
	return func(c *Config) error {
		c.process = process
		return nil
	}
}

func WithReCompile(b bool) Option {
	return func(c *Config) error {
		c.SetCompileReCompile(b)
		return nil
	}
}

func WithStrictMode(b bool) Option {
	return func(c *Config) error {
		c.SetCompileStrictMode(b)
		return nil
	}
}

func WithLocalFs(path string) Option {
	return func(c *Config) error {
		WithConfigInfo(map[string]any{
			"kind":       "local",
			"local_file": path,
		})(c)
		return nil
	}
}

func WithFileSystem(fs fi.FileSystem) Option {
	return func(c *Config) error {
		if fs == nil {
			return utils.Errorf("need set filesystem")
		}
		c.fs = getUnifiedSeparatorFs(fs)
		return nil
	}
}

func WithConfigInfoRaw(info string) Option {
	return func(c *Config) error {
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
	return func(c *Config) error {
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
		return func(*Config) error { return nil }
	}
	if language, err := consts.ValidateLanguage(input_language); err == nil {
		return WithLanguage(language)
	} else {
		return func(c *Config) error {
			return err
		}
	}
}

func WithLanguage(language consts.Language) Option {
	return func(c *Config) error {
		if language == "" {
			return nil
		}
		c.language = language
		if create, ok := LanguageBuilderCreater[language]; ok {
			c.LanguageBuilder = create()
		} else {
			log.Errorf("SSA not support language %s", language)
			c.LanguageBuilder = nil
		}
		return nil
	}
}

func WithFileSystemEntry(files ...string) Option {
	return func(c *Config) error {
		c.entryFile = append(c.entryFile, files...)
		return nil
	}
}

func WithProgramPath(path string) Option {
	return func(c *Config) error {
		c.programPath = path
		return nil
	}
}

func WithIncludePath(path ...string) Option {
	return func(c *Config) error {
		c.includePath = append(c.includePath, path...)
		return nil
	}
}

func WithExternLib(name string, table map[string]any) Option {
	return func(c *Config) error {
		c.externLib[name] = table
		return nil
	}
}

func WithExternValue(table map[string]any) Option {
	return func(c *Config) error {
		for name, value := range table {
			c.externValue[name] = value
		}
		return nil
	}
}

func WithExternMethod(b ssa.MethodBuilder) Option {
	return func(c *Config) error {
		c.externMethod = b
		return nil
	}
}

func WithExternBuildValueHandler(id string, callback func(b *ssa.FunctionBuilder, id string, v any) ssa.Value) Option {
	return func(c *Config) error {
		if c.externBuildValueHandler == nil {
			c.externBuildValueHandler = make(map[string]func(b *ssa.FunctionBuilder, id string, v any) ssa.Value)
		}
		c.externBuildValueHandler[id] = callback
		return nil
	}
}

func WithPeepholeSize(size int) Option {
	return func(c *Config) error {
		c.SetCompilePeepholeSize(size)
		return nil
	}
}

func WithIgnoreSyntaxError(b ...bool) Option {
	return func(c *Config) error {
		if len(b) > 0 {
			c.ignoreSyntaxErr = b[0]
		} else {
			c.ignoreSyntaxErr = true
		}
		return nil
	}
}

func WithExternInfo(info string) Option {
	return func(c *Config) error {
		c.externInfo = info
		return nil
	}
}

func WithDefineFunc(table map[string]any) Option {
	return func(c *Config) error {
		for name, t := range table {
			c.defineFunc[name] = t
		}
		return nil
	}
}

func WithFeedCode(b ...bool) Option {
	return func(c *Config) error {
		if len(b) > 0 {
			c.feedCode = b[0]
		} else {
			c.feedCode = true
		}
		return nil
	}
}

func WithProgramDescription(desc string) Option {
	return func(c *Config) error {
		c.ProgramDescription = desc
		return nil
	}
}

// save to database, please set the program name
func WithProgramName(name string) Option {
	return func(c *Config) error {
		c.ProgramName = name
		c.databaseKind = ssa.ProgramCacheDBWrite
		return nil
	}
}

func WithProjectName(name string) Option {
	return func(c *Config) error {
		project, err := ssaproject.LoadSSAProjectBuilderByName(name)
		if err != nil {
			return err
		}
		sc := project.Config
		c.ProjectName = name
		if sc == nil || sc.SSACompile == nil {
			return utils.Errorf("project %s config not found", name)
		}
		return WithSSAConfig(sc)(c)
	}
}

func WithMemory(ttl ...time.Duration) Option {
	return func(c *Config) error {
		c.databaseKind = ssa.ProgramCacheMemory
		if len(ttl) > 0 {
			c.programSaveTTL = ttl[0]
		}
		return nil
	}
}

func WithSSAConfig(sc *ssaconfig.Config) Option {
	return func(c *Config) error {
		if sc != nil {
			c.Config = sc
		}
		if sc.GetCompileMemory() {
			err := WithMemory()(c)
			if err != nil {
				return err
			}
		}
		if sc.GetCompileStrictMode() {
			err := WithStrictMode(true)(c)
			if err != nil {
				return err
			}
		}
		if sc.GetCompilePeepholeSize() > 0 {
			err := WithPeepholeSize(sc.GetCompilePeepholeSize())(c)
			if err != nil {
				return err
			}
		}
		if sc.GetCompileExcludeFiles() != nil {
			_, err := DefaultExcludeFunc(sc.GetCompileExcludeFiles())
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func WithConcurrency(concurrency int) Option {
	return func(c *Config) error {
		c.SetCompileConcurrency(uint32(concurrency))
		return nil
	}
}

func WithDatabaseProgramCacheHitter(h func(i any)) Option {
	return func(c *Config) error {
		c.DatabaseProgramCacheHitter = h
		return nil
	}
}

func WithEnableCache(b ...bool) Option {
	return func(c *Config) error {
		if len(b) > 0 {
			c.EnableCache = b[0]
		} else {
			c.EnableCache = true
		}
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(c *Config) error {
		c.ctx = ctx
		return nil
	}
}

func getUnifiedSeparatorFs(fs fi.FileSystem) fi.FileSystem {
	return filesys.NewUnifiedFS(fs,
		filesys.WithUnifiedFsSeparator(ssadb.IrSourceFsSeparators),
	)
}

var ttlSSAParseCache = createCache(30 * time.Minute)

type programResult struct {
	prog *Program
	err  error
}

func createCache(ttl time.Duration) *utils.CacheWithKey[string, *programResult] {
	cache := utils.NewTTLCacheWithKey[string, *programResult](ttl)
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
	config, err := DefaultConfig(opts...)
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
		// Use single-flight behavior to ensure only one parsing operation per hash
		result, err := ttlSSAParseCache.GetOrLoad(hash, func() (*programResult, error) {
			ret, err := config.parseFile()
			return &programResult{
				prog: ret,
				err:  err,
			}, nil
		})
		if err != nil {
			return nil, err
		}
		return result.prog, result.err
	} else {
	}

	ret, err := config.parseFile()
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
	// "NewRiskCompare":     NewSSAComparator[*schema.SSARisk],
	// "NewRiskCompareItem": NewSSARiskComparisonItem,

	"withProjectName": WithProjectName,

	"withConcurrency":        WithConcurrency,
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
	"withMemory":             WithMemory,
	"withSSAConfig":          WithSSAConfig,

	//diff compare
	// "withDiffProgName":          DiffWithProgram,
	// "withDiffRuleName":          DiffWithRuleName,
	// "withDiffVariableName":      DiffWithVariableName,
	// "withDiffRuntimeId":         DiffWithRuntimeId,
	// "withGenerateHash":          WithSSARiskComparisonInfoGenerate,
	// "withCompareResultCallback": WithSSARiskDiffResultHandler,
	// "withDefaultRiskSave":       WithSSARiskDiffSaveResultHandler,
	//diff compare kind
	"progName":  schema.Program,
	"runtimeId": schema.RuntimeId,

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
