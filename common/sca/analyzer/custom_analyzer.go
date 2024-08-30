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
