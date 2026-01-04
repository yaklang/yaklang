package ssaapi

import (
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// needsSeparatorConversion 判断文件系统是否需要路径分隔符转换
//
// 不需要转换的文件系统（固定使用 '/' 或已包装）：
//   - ZipFS (固定使用 '/')
//   - JarFS (包装 ZipFS，继承 '/' 分隔符)
//   - embedFs (固定使用 '/')
//   - VirtualFS (固定使用 '/')
//   - UnifiedFS (已包装，避免重复包装)
//
// 需要转换的文件系统（分隔符不是 '/'）：
//   - LocalFs (Windows: '\', Linux/Mac: '/')
//   - RelLocalFs (Windows: '\', Linux/Mac: '/')
//   - HookFS (取决于底层文件系统)
//   - ExpandedZipFS (取决于底层文件系统)
//   - 其他未列出的类型
//
// 判断逻辑：
//   1. 如果文件系统是 nil，不需要转换
//   2. 使用 switch 检查特定类型，这些类型不需要转换
//   3. 其他类型需要转换
func needsSeparatorConversion(fs fi.FileSystem) bool {
	if fs == nil {
		return false
	}

	// 使用 switch 方式检查文件系统类型
	switch fs.(type) {
	case *filesys.ZipFS:
		// ZipFS 固定使用 '/'，不需要转换
		return false
	case *javaclassparser.JarFS:
		// JarFS 包装 ZipFS，继承 '/' 分隔符，不需要转换
		return false
	case *filesys.VirtualFS:
		// VirtualFS 固定使用 '/'，不需要转换
		return false
	case *filesys.UnifiedFS:
		// UnifiedFS 已包装，避免重复包装
		return false
	default:
		// 其他类型需要转换
		return true
	}
}

func getUnifiedSeparatorFs(fs fi.FileSystem) fi.FileSystem {
	// 只在需要转换时才包装
	if needsSeparatorConversion(fs) {
		return filesys.NewUnifiedFS(fs,
			filesys.WithUnifiedFsSeparator(ssadb.IrSourceFsSeparators),
		)
	}
	// 不需要转换，直接返回原文件系统
	return fs
}

var ttlSSAParseCache = createCache(30 * time.Minute)
var parseSingleFlightCache = utils.NewSingleFlightCache(ttlSSAParseCache.CacheExWithKey)

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
		result, err := parseSingleFlightCache.Do(hash, func() (*programResult, error) {
			ret, err := config.parseFile()
			return &programResult{
				prog: ret,
				err:  err,
			}, err
		})
		if err != nil {
			return nil, err
		}
		return result.prog, result.err
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
	"withExcludeFile":        WithExcludeFunc,
	"withDefaultExcludeFunc": WithExcludeFunc, // deprecated, use withExcludeFile instead
	"withMemory":             WithMemory,
	"withFilePerformanceLog": WithFilePerformanceLog,

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
