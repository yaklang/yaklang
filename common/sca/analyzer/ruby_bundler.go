package analyzer

import (
	"github.com/aquasecurity/go-dep-parser/pkg/ruby/bundler"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypRubyBundler    TypAnalyzer = "ruby-bundler-lang"
	statusRubyBundler int         = 1
	GemLock                       = "Gemfile.lock"
)

func init() {
	RegisterAnalyzer(TypRubyBundler, NewRubyBundlerAnalyzer())
}

type rubyBunlderAnalyzer struct{}

func NewRubyBundlerAnalyzer() *rubyBunlderAnalyzer {
	return &rubyBunlderAnalyzer{}
}

func (a rubyBunlderAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusRubyBundler:
		pkgs, err := ParseLanguageConfiguration(fi, bundler.NewParser())
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a rubyBunlderAnalyzer) Match(info MatchInfo) int {
	if info.fi.Name() == GemLock {
		return statusRubyBundler
	}
	return 0
}
