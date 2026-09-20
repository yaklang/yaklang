package sca

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/yaklang/yaklang/common/sca/internal/fsio"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"io/fs"
	"os"
)

type ScanConfig struct {
	limits   ResourceLimits
	snapshot string

	numWorkers      int
	scanMode        analyzer.ScanMode
	usedAnalyzers   []analyzer.TypAnalyzer
	customAnalyzers []analyzer.Analyzer
	fs              fi.FileSystem
}

type ScanOption func(*ScanConfig)

func NewConfig() *ScanConfig {
	return &ScanConfig{
		numWorkers:    5,
		scanMode:      analyzer.AllMode,
		usedAnalyzers: []analyzer.TypAnalyzer{},
	}
}

// customAnalyzer 注册一个自定义 SCA 分析器，通过 matchFunc 决定是否处理某文件、analyzeFunc 产出软件包结果
// 在 yak 中通过 sca.customAnalyzer 调用
// 参数:
//   - matchFunc: 文件匹配函数，返回非 0 表示该文件由本分析器处理
//   - analyzeFunc: 分析函数，返回识别到的自定义软件包列表
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：注册一个自定义分析器
// opt = sca.customAnalyzer(func(info) { return 0 }, func(fi, others) { return [] })
// ```
func _withCustomAnalyzer(matchFunc func(info analyzer.MatchInfo) int, analyzeFunc func(fi *analyzer.FileInfo, otherFi map[string]*analyzer.FileInfo) []*analyzer.CustomPackage) ScanOption {
	return func(c *ScanConfig) {
		c.customAnalyzers = append(c.customAnalyzers, analyzer.NewCustomAnalyzer(matchFunc, analyzeFunc))
	}
}

// scanMode 设置扫描模式，控制识别全部成分、仅系统包或仅语言依赖
// 在 yak 中通过 sca.scanMode 调用，取值如 sca.MODE_ALL、sca.MODE_PKG、sca.MODE_LANGUAGE
// 参数:
//   - mode: 扫描模式
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：仅扫描语言依赖
// pkgs = sca.ScanLocalFilesystem("/path/to/project", sca.scanMode(sca.MODE_LANGUAGE))~
// ```
func _withScanMode(mode analyzer.ScanMode) ScanOption {
	return func(c *ScanConfig) {
		c.scanMode |= mode
	}
}

// concurrent 设置扫描时的并发 worker 数量
// 在 yak 中通过 sca.concurrent 调用
// 参数:
//   - n: 并发 worker 数量
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：以 10 并发扫描本地目录
// pkgs = sca.ScanLocalFilesystem("/path/to/project", sca.concurrent(10))~
// ```
func _withConcurrent(n int) ScanOption {
	return func(c *ScanConfig) {
		c.numWorkers = n
	}
}

// analyzers 指定本次扫描启用的分析器类型，仅运行所列分析器
// 在 yak 中通过 sca.analyzers 调用，取值如 sca.ANALYZER_TYPE_JAVA_POM、sca.ANALYZER_TYPE_NODE_NPM
// 参数:
//   - a: 一个或多个分析器类型
//
// 返回值:
//   - 一个扫描配置选项
//
// Example:
// ```
// // 该示例为示意性用法：仅启用 Java POM 分析器
// pkgs = sca.ScanLocalFilesystem("/path/to/project", sca.analyzers(sca.ANALYZER_TYPE_JAVA_POM))~
// ```
func _withAnalayzers(a ...analyzer.TypAnalyzer) ScanOption {
	return func(c *ScanConfig) {
		c.usedAnalyzers = append(c.usedAnalyzers, a...)
	}
}

func scanFS(_ string, c *ScanConfig) ([]*dxtypes.Package, error) {
	p, _, e := scanPipeline(context.Background(), c.fs, c)
	return p, e
}

// ScanLocalFilesystem 扫描本地文件系统目录，识别其中的软件成分(SCA)，返回检测到的软件包列表
// 在 yak 中通过 sca.ScanLocalFilesystem 调用，会根据各类包管理器清单(如 package.json、go.mod 等)解析依赖
// 参数:
//   - p: 待扫描的本地目录路径
//   - opts: 可选配置项，如 sca.concurrent、sca.scanMode、sca.analyzers
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：扫描本地项目目录的软件成分
// pkgs = sca.ScanLocalFilesystem("/path/to/project")~
//
//	for pkg = range pkgs {
//	    println(pkg.Name, pkg.Version)
//	}
//
// ```
func ScanLocalFilesystem(p string, opts ...ScanOption) ([]*dxtypes.Package, error) {
	config := NewConfig()
	for _, opt := range opts {
		opt(config)
	}
	config.fs = fsio.New(os.DirFS(p))
	if config.numWorkers < 1 || config.numWorkers > 64 {
		return nil, fmt.Errorf("worker count must be between 1 and 64")
	}
	return scanFS(p, config)
}

// ScanFilesystem 扫描给定的文件系统接口对象，识别其中的软件成分(SCA)，返回检测到的软件包列表
// 在 yak 中通过 sca.ScanFilesystem 调用，可配合 filesys 包构造的各类文件系统使用
// 参数:
//   - p: 实现 FileSystem 接口的文件系统对象(如 fsio.New(os.DirFS(".")) 等)
//   - opts: 可选配置项
//
// 返回值:
//   - 检测到的软件包列表
//   - 错误信息，输入文件系统为空或扫描失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：扫描任意文件系统接口对象
// fs = fsio.New(os.DirFS("."))
// pkgs = sca.ScanFilesystem(fs)~
// println("packages:", len(pkgs))
// ```
func ScanFilesystem(p fs.FS, opts ...ScanOption) ([]*dxtypes.Package, error) {
	config := NewConfig()
	if p == nil {
		return nil, fmt.Errorf("nil input filesystem")
	}
	config.fs = fsio.New(p)
	for _, opt := range opts {
		opt(config)
	}
	if config.fs == nil {
		return nil, fmt.Errorf("ScanFilesystem need fs.FS interface as input, try filesys.New... instead: %T", p)
	}
	if config.numWorkers < 1 || config.numWorkers > 64 {
		return nil, fmt.Errorf("worker count must be between 1 and 64")
	}
	return scanFS(".", config)
}
