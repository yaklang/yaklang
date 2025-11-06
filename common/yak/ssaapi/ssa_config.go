package ssaapi

import (
	"context"
	"fmt"
	"time"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

type ProcessFunc func(msg string, process float64)

type Config struct {
	*ssaconfig.Config // config

	// language
	LanguageBuilder ssa.Builder

	// other compile ssaconfig.Options
	feedCode        bool
	ignoreSyntaxErr bool

	// input, code or project path
	originEditor *memedit.MemEditor
	// file system
	fs          fi.FileSystem
	entryFile   []string
	programPath string
	includePath []string

	databaseKind ssa.ProgramCacheKind
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

func (c *Config) CalcHash() string {
	return utils.CalcSha1(c.originEditor.GetSourceCode(), c.GetLanguage(), c.ignoreSyntaxErr, c.externInfo)
}

var WithConfigInfo = ssaconfig.WithCodeSourceMap

var withExcludeFile = ssaconfig.SetOption("ssa_compile/exclude_file", func(c *Config, v func(path, filename string) bool) {
	c.excludeFile = v
})

func WithExcludeFunc(patterns []string) ssaconfig.Option {
	var compile []glob.Glob
	for _, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil
		}
		compile = append(compile, g)
	}
	return withExcludeFile(func(dir string, path string) bool {
		for _, g := range compile {
			if match := g.Match(dir); match {
				return true
			}
			if match := g.Match(path); match {
				return true
			}
		}
		return false
	})
}

func (c *Config) Processf(process float64, format string, arg ...any) {
	msg := fmt.Sprintf(format, arg...)
	if c.process != nil {
		c.process(msg, process)
	} else {
		log.Info(msg, process)
	}
}

var WithASTOrder = ssaconfig.SetOption("ssa_compile/ast_order", func(c *Config, v ssareducer.ASTSequenceType) {
	c.astSequence = v
})

var WithLogLevel = ssaconfig.SetOption("ssa_compile/log_level", func(c *Config, v string) {
	c.logLevel = v
	log.SetLevel(v)
})

var WithCacheTTL = ssaconfig.SetOption("ssa_compile/cache_ttl", func(c *Config, v time.Duration) {
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

var WithFileSystem = ssaconfig.SetOption("ssa_compile/file_system", func(c *Config, fs fi.FileSystem) {
	if fs == nil {
		return
	}
	c.fs = getUnifiedSeparatorFs(fs)
})

var WithRawLanguage = ssaconfig.WithProjectRawLanguage
var WithLanguage = ssaconfig.WithProjectLanguage

var withFileSystemEntry = ssaconfig.SetOption("ssa_compile/file_system_entry", func(c *Config, v []string) {
	c.entryFile = append(c.entryFile, v...)
})

func WithFileSystemEntry(v ...string) ssaconfig.Option {
	return withFileSystemEntry(v)
}

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
}

var WithExternValue = ssaconfig.SetOption("ssa_compile/extern_value", func(c *Config, table map[string]any) {
	for name, value := range table {
		c.externValue[name] = value
	}
})

var WithExternMethod = ssaconfig.SetOption("ssa_compile/extern_method", func(c *Config, b ssa.MethodBuilder) {
	c.externMethod = b
})

func WithExternBuildValueHandler(id string, callback func(b *ssa.FunctionBuilder, id string, v any) ssa.Value) ssaconfig.Option {
	return withExternBuildValueHandler(struct {
		id       string
		callback func(b *ssa.FunctionBuilder, id string, v any) ssa.Value
	}{
		id:       id,
		callback: callback,
	})
}

var withExternBuildValueHandler = ssaconfig.SetOption("ssa_compile/extern_build_value_handler", func(
	c *Config, a struct {
		id       string
		callback func(b *ssa.FunctionBuilder, id string, v any) ssa.Value
	}) {
	if c.externBuildValueHandler == nil {
		c.externBuildValueHandler = make(map[string]func(b *ssa.FunctionBuilder, id string, v any) ssa.Value)
	}
	c.externBuildValueHandler[a.id] = a.callback
})

var WithPeepholeSize = ssaconfig.WithCompilePeepholeSize

var WithIgnoreSyntaxError = ssaconfig.SetOption("ssa_compile/ignore_syntax_error", func(c *Config, v bool) {
	c.ignoreSyntaxErr = v
})

var WithExternInfo = ssaconfig.SetOption("ssa_compile/extern_info", func(c *Config, v string) {
	c.externInfo = v
})

var WithDefineFunc = ssaconfig.SetOption("ssa_compile/define_func", func(c *Config, table map[string]any) {
	for name, t := range table {
		c.defineFunc[name] = t
	}
})

var WithFeedCode = ssaconfig.SetOption("ssa_compile/feed_code", func(c *Config, v bool) {
	c.feedCode = v
})

var WithProgramDescription = ssaconfig.WithProgramDescription

var WithProgramName = ssaconfig.WithProgramNames

var WithMemory = ssaconfig.WithCompileMemoryCompile

var WithConcurrency = ssaconfig.WithCompileConcurrency

var WithDatabaseProgramCacheHitter = ssaconfig.SetOption("ssa_compile/database_program_cache_hitter", func(c *Config, h func(i any)) {
	c.DatabaseProgramCacheHitter = h
})

var withEnableCache = ssaconfig.SetOption("ssa_compile/enable_cache", func(c *Config, v bool) {
	c.EnableCache = v
})

func WithEnableCache(b ...bool) ssaconfig.Option {
	enable := true // default true
	if len(b) > 0 {
		enable = b[0]
	}
	return withEnableCache(enable)
}

var WithEditor = ssaconfig.SetOption("ssa_compile/editor", func(c *Config, v *memedit.MemEditor) {
	c.originEditor = v
})

var WithContext = ssaconfig.WithContext

func DefaultConfig(opts ...ssaconfig.Option) (*Config, error) {
	sc, err := ssaconfig.New(ssaconfig.ModeSSACompile, opts...)
	if err != nil {
		return nil, err
	}
	c := &Config{
		LanguageBuilder:            nil,
		programPath:                ".",
		entryFile:                  make([]string, 0),
		cacheTTL:                   make([]time.Duration, 0),
		externLib:                  make(map[string]map[string]any),
		externValue:                make(map[string]any),
		defineFunc:                 make(map[string]any),
		DatabaseProgramCacheHitter: func(any) {},
		ctx:                        sc.GetContext(),
		excludeFile: func(path, filename string) bool {
			return false
		},
		logLevel:    "error",
		astSequence: ssareducer.OutOfOrder,
		Config:      sc,
	}
	ssaconfig.ApplyExtraOptions(c, c.Config)

	if fs, err := c.parseFSFromInfo(); err != nil {
		return nil, err
	} else if fs != nil {
		c.fs = fs
	}
	switch c.databaseKind {
	case ssa.ProgramCacheNone:
		c.databaseKind = ssa.ProgramCacheMemory
		if c.GetProgramName() != "" {
			// if set program name, use db write
			c.databaseKind = ssa.ProgramCacheDBWrite
		}
		if c.GetCompileMemory() {
			// if set enable memory, use memory force
			c.databaseKind = ssa.ProgramCacheMemory
		}
	}

	// memory/write mode no need check source code
	if c.fs == nil && c.originEditor == nil {
		return nil, utils.Errorf("Compile Proram should set file system or origin editor ")
	}

	if c.GetLanguage() != "" {
		if create, ok := LanguageBuilderCreater[c.GetLanguage()]; ok {
			c.LanguageBuilder = create()
		}
	}
	return c, nil
}

func WithLocalFs(path string) ssaconfig.Option {
	return WithConfigInfo(map[string]any{
		"kind":       "local",
		"local_file": path,
	})
}
