package ssaapi

import (
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

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
func Parse(code string, opts ...ssaconfig.Option) (*Program, error) {
	input := strings.NewReader(code)
	return ParseFromReader(input, opts...)
}

// ParseFromReader parse simple file to ssa.Program
func ParseFromReader(input io.Reader, opts ...ssaconfig.Option) (*Program, error) {
	if input != nil {
		raw, err := io.ReadAll(input)
		if err != nil {
			log.Warnf("read input error: %v", err)
		}
		// config.originEditor = memedit.NewMemEditor(string(raw))
		opts = append(opts, WithEditor(memedit.NewMemEditor(string(raw))))
	}
	config, err := DefaultConfig(opts...)
	if err != nil {
		return nil, err
	}

	if config.EnableCache {
		hash := config.CalcHash()
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
		return utils.Errorf("not support language %s", p.config.GetLanguage())
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

	"withProjectName": ssaconfig.WithProjectName,

	"withConcurrency":        WithConcurrency,
	"withLanguage":           WithRawLanguage,
	"withConfigInfo":         ssaconfig.WithCodeSourceMap,
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
	"withDefaultExcludeFunc": WithExcludeFunc,
	"withMemory":             WithMemory,
	"withSSAConfig":          WithSSAConfig,
	"withJSONConfig":         WithJSONConfig,
	// language:
	"Javascript": ssaconfig.JS,
	"Yak":        ssaconfig.Yak,
	"PHP":        ssaconfig.PHP,
	"Java":       ssaconfig.JAVA,

	/// static analyze
	"YaklangScriptChecking": YaklangScriptChecking,

	// result
	"NewResultFromDB": LoadResultByID,
}
