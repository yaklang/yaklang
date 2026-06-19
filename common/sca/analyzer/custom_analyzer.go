package analyzer

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

type customAnalyzer struct {
	matchFunc   func(info MatchInfo) int
	analyzeFunc func(fi *FileInfo, otherFi map[string]*FileInfo) []*CustomPackage
}

var _ Analyzer = (*customAnalyzer)(nil)

func NewCustomAnalyzer(matchFunc func(info MatchInfo) int, analyzeFunc func(fi *FileInfo, otherFi map[string]*FileInfo) []*CustomPackage) *customAnalyzer {
	return &customAnalyzer{
		matchFunc:   matchFunc,
		analyzeFunc: analyzeFunc,
	}
}

// NewAnalyzerResult 创建一个自定义分析结果(软件包)，用于在自定义 SCA 分析器中返回识别到的组件
// 在 yak 中通过 sca.NewAnalyzerResult 调用
// 参数:
//   - name: 软件包名称
//   - version: 软件包版本号
//
// 返回值:
//   - 包含名称与版本的自定义软件包对象
//
// Example:
// ```
// pkg = sca.NewAnalyzerResult("openssl", "1.1.1w")
// println(pkg.Name)      // OUT: openssl
// println(pkg.Version)   // OUT: 1.1.1w
// assert pkg.Name == "openssl", "package name should be set"
// assert pkg.Version == "1.1.1w", "package version should be set"
// ```
func NewAnalyzerResult(name, version string) *CustomPackage {
	return &CustomPackage{
		Name:    name,
		Version: version,
	}
}

func (a customAnalyzer) Match(info MatchInfo) int {
	return a.matchFunc(info)
}

func (a customAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	packages := a.analyzeFunc(fi, afi.MatchedFileInfos)
	if len(packages) == 0 {
		return nil, nil
	}
	return lo.Map(packages, func(p *CustomPackage, _ int) *dxtypes.Package {
		return &dxtypes.Package{
			Name:           p.Name,
			Version:        p.Version,
			IsVersionRange: p.IsVersionRange,
		}
	}), nil
}

type CustomPackage struct {
	Name           string
	Version        string
	IsVersionRange bool
}
