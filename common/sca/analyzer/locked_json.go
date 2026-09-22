package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/lockjson"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypNuget TypAnalyzer = "nuget-lang"
	TypSwift TypAnalyzer = "swift-lang"
)

type lockedJSONAnalyzer struct {
	filename string
	parser   types.Parser
}

func init() {
	RegisterAnalyzer(TypNuget, NewNugetAnalyzer())
	RegisterAnalyzer(TypSwift, NewSwiftAnalyzer())
}
func (a *lockedJSONAnalyzer) Match(info MatchInfo) int {
	if info.FileInfo.Name() == a.filename {
		return 1
	}
	return 0
}
func (a *lockedJSONAnalyzer) Analyze(info AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	return ParseLanguageConfiguration(info.Self, a.parser)
}

func NewNugetAnalyzer() *lockedJSONAnalyzer {
	return &lockedJSONAnalyzer{"packages.lock.json", lockjson.Nuget{}}
}
func NewSwiftAnalyzer() *lockedJSONAnalyzer {
	return &lockedJSONAnalyzer{"Package.resolved", lockjson.Swift{}}
}
