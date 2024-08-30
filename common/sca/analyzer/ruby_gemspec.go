package analyzer

import (
	"path/filepath"
	"regexp"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/ruby/gemspec"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypRubyGemSpec TypAnalyzer = "ruby-gemspec-lang"
	statusGemSpec  int         = 1
)

var gemspecRegex = regexp.MustCompile(`.*/specifications/.+\.gemspec`)

func init() {
	RegisterAnalyzer(TypRubyGemSpec, NewRubyGemSpecAnalyzer())
}

type rubyGemSpecAnalyzer struct{}

func NewRubyGemSpecAnalyzer() *rubyGemSpecAnalyzer {
	return &rubyGemSpecAnalyzer{}
}

func (a rubyGemSpecAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusGemSpec:
		pkgs, err := ParseLanguageConfiguration(fi, gemspec.NewParser())
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a rubyGemSpecAnalyzer) Match(info MatchInfo) int {
	if gemspecRegex.MatchString(filepath.ToSlash(info.Path)) {
		return statusGemSpec
	}
	return 0
}
