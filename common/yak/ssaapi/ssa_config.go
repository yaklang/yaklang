package ssaapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

type ProcessFunc func(msg string, process float64)

type Config struct {
	*ssaconfig.Config // config

	databaseKind   ssa.ProgramCacheKind
	programSaveTTL time.Duration
	// program
	ProgramName        string
	ProgramDescription string

	// language
	language        ssaconfig.Language
	LanguageBuilder ssa.Builder

	// other compile ssaconfig.Options
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

	// other build ssaconfig.Options
	DatabaseProgramCacheHitter func(any)
	EnableCache                bool
	// for hash
	externInfo string
	// process ctx
	ctx context.Context

	excludeFile func(path, filename string) bool

	logLevel string

	astSequence ssareducer.ASTSequenceType
}

const (
	enableCache = "ssa_compile/enable_cache"
	astOrder    = "ssa_compile/ast_order"
	cacheTTL    = "ssa_compile/cache_ttl"
	logLevel    = "ssa_compile/log_level"
)

func DefaultExcludeFunc(patterns []string) (ssaconfig.Option, error) {
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

func (c *Config) Processf(process float64, format string, arg ...any) {
	msg := fmt.Sprintf(format, arg...)
	if c.process != nil {
		c.process(msg, process)
	} else {
		log.Info(msg, process)
	}
}

var WithAstOrder = ssaconfig.SetOption(astOrder, func(c *Config, v ssareducer.ASTSequenceType) {
	c.astSequence = v
})

var WithLogLevel = ssaconfig.SetOption(logLevel, func(c *Config, v string) {
	c.logLevel = v
	log.SetLevel(v)
})

var WithCacheTTL = ssaconfig.SetOption(cacheTTL, func(c *Config, v time.Duration) {
	c.cacheTTL = append(c.cacheTTL, v)
})

var WithExcludeFile = ssaconfig.SetOption("ssa_compile/exclude_file", func(c *Config, v func(path, filename string) bool) {
	c.excludeFile = v
})

var WithProcess = ssaconfig.SetOption("ssa_compile/process", func(c *Config, v ProcessFunc) {
	c.process = v
})

var WithReCompile = ssaconfig.WithCompileReCompile

var WithStrictMode = ssaconfig.WithCompileStrictMode

func WithLocalFs(path string) ssaconfig.Option {
	return func(c *Config) error {
		WithConfigInfo(map[string]any{
			"kind":       "local",
			"local_file": path,
		})(c)
		return nil
	}
}

func WithFileSystem(fs fi.FileSystem) ssaconfig.Option {
	return func(c *Config) error {
		if fs == nil {
			return utils.Errorf("need set filesystem")
		}
		c.fs = getUnifiedSeparatorFs(fs)
		return nil
	}
}

func WithConfigInfoRaw(info string) ssaconfig.Option {
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

func WithConfigInfo(input map[string]any) ssaconfig.Option {
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

var WithRawLanguage = ssaconfig.WithProjectRawLanguage
var WithLanguage = ssaconfig.WithProjectLanguage

// func WithLanguage(language ssaconfig.Language) ssaconfig.Option {
// 	return func(c *Config) error {
// 		if language == "" {
// 			return nil
// 		}
// 		c.language = language
// 		if create, ok := LanguageBuilderCreater[language]; ok {
// 			c.LanguageBuilder = create()
// 		} else {
// 			log.Errorf("SSA not support language %s", language)
// 			c.LanguageBuilder = nil
// 		}
// 		return nil
// 	}
// }

var WithFileSystemEntry = ssaconfig.SetOption("ssa_compile/file_system_entry", func(c *Config, v []string) {
	c.entryFile = append(c.entryFile, v...)
})

var WithProgramPath = ssaconfig.SetOption("ssa_compile/program_path", func(c *Config, v string) {
	c.programPath = v
})

var WithIncludePath = ssaconfig.SetOption("ssa_compile/include_path", func(c *Config, v []string) {
	c.includePath = append(c.includePath, v...)
})

var withExternLib = ssaconfig.SetOption("ssa_compile/extern_lib", func(
	c *Config, a struct {
		name  string
		table map[string]any
	}) {
	c.externLib[a.name] = a.table
})

func WithExternLib(name string, table map[string]any) ssaconfig.Option {
	return func(c *ssaconfig.Config) error {
		withExternLib(struct {
			name  string
			table map[string]any
		}{
			name:  name,
			table: table,
		})(c)
		return nil
	}
	//	return func(c *Config) error {
	//		c.externLib[name] = table
	//		return nil
	//	}
}

func WithExternValue(table map[string]any) ssaconfig.Option {
	return func(c *Config) error {
		for name, value := range table {
			c.externValue[name] = value
		}
		return nil
	}
}

func WithExternMethod(b ssa.MethodBuilder) ssaconfig.Option {
	return func(c *Config) error {
		c.externMethod = b
		return nil
	}
}

func WithExternBuildValueHandler(id string, callback func(b *ssa.FunctionBuilder, id string, v any) ssa.Value) ssaconfig.Option {
	return func(c *Config) error {
		if c.externBuildValueHandler == nil {
			c.externBuildValueHandler = make(map[string]func(b *ssa.FunctionBuilder, id string, v any) ssa.Value)
		}
		c.externBuildValueHandler[id] = callback
		return nil
	}
}

func WithPeepholeSize(size int) ssaconfig.Option {
	return func(c *Config) error {
		c.SetCompilePeepholeSize(size)
		return nil
	}
}

func WithIgnoreSyntaxError(b ...bool) ssaconfig.Option {
	return func(c *Config) error {
		if len(b) > 0 {
			c.ignoreSyntaxErr = b[0]
		} else {
			c.ignoreSyntaxErr = true
		}
		return nil
	}
}

func WithExternInfo(info string) ssaconfig.Option {
	return func(c *Config) error {
		c.externInfo = info
		return nil
	}
}

func WithDefineFunc(table map[string]any) ssaconfig.Option {
	return func(c *Config) error {
		for name, t := range table {
			c.defineFunc[name] = t
		}
		return nil
	}
}

func WithFeedCode(b ...bool) ssaconfig.Option {
	return func(c *Config) error {
		if len(b) > 0 {
			c.feedCode = b[0]
		} else {
			c.feedCode = true
		}
		return nil
	}
}

func WithProgramDescription(desc string) ssaconfig.Option {
	return func(c *Config) error {
		c.ProgramDescription = desc
		return nil
	}
}

// save to database, please set the program name
func WithProgramName(name string) ssaconfig.Option {
	return func(c *Config) error {
		c.ProgramName = name
		c.databaseKind = ssa.ProgramCacheDBWrite
		return nil
	}
}

func WithMemory(ttl ...time.Duration) ssaconfig.Option {
	return func(c *Config) error {
		c.databaseKind = ssa.ProgramCacheMemory
		if len(ttl) > 0 {
			c.programSaveTTL = ttl[0]
		}
		return nil
	}
}

func WithSSAConfig(sc *ssaconfig.Config) ssaconfig.Option {
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

func WithConcurrency(concurrency int) ssaconfig.Option {
	return func(c *Config) error {
		c.SetCompileConcurrency(uint32(concurrency))
		return nil
	}
}

func WithDatabaseProgramCacheHitter(h func(i any)) ssaconfig.Option {
	return func(c *Config) error {
		c.DatabaseProgramCacheHitter = h
		return nil
	}
}

var withEnableCache = ssaconfig.SetOption(enableCache, func(c *Config, v bool) {
	c.EnableCache = v
})

func WithEnableCache(b ...bool) ssaconfig.Option {
	return func(c *ssaconfig.Config) error {
		enable := true // default true
		if len(b) > 0 {
			enable = b[0]
		}
		withEnableCache(enable)
		return nil
	}
}

var WithContext = ssaconfig.WithContext

func DefaultConfig(opts ...ssaconfig.Option) (*Config, error) {
	sc, err := ssaconfig.New(ssaconfig.ModeSSACompile)
	if err != nil {
		return nil, err
	}
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
		if err := opt(c.Config); err != nil {
			return nil, err
		}
	}
	ssaconfig.ApplyExtrassaconfig.Options(c, c.Config)
	return c, nil
}
