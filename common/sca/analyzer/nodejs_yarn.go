package analyzer

import (
	"github.com/aquasecurity/go-dep-parser/pkg/nodejs/yarn"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypNodeYarn TypAnalyzer = "yarm-lang"

	YarnLock       = "yarn.lock"
	yarnLockStatus = 1
)

type yarnAnalyzer struct{}

func init() {
	RegisterAnalyzer(TypNodeYarn, NewNodeYarnAnalyzer())
}

func NewNodeYarnAnalyzer() *yarnAnalyzer {
	return &yarnAnalyzer{}
}

func (a yarnAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case yarnLockStatus:
		pkgs, err := ParseLanguageConfiguration(fi, yarn.NewParser())
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a yarnAnalyzer) Match(info MatchInfo) int {
	if info.fi.Name() == YarnLock {
		return yarnLockStatus
	}
	return 0
}
