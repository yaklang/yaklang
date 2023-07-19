package analyzer

import (
	"github.com/aquasecurity/go-dep-parser/pkg/nodejs/pnpm"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypNodePnpm TypAnalyzer = "npmp-lang"

	pnpmLockYaml = "pnpm-lock.yaml"

	pnpmLockStatus = 1
)

type pnpmAnalyzer struct{}

func init() {
	RegisterAnalyzer(TypNodePnpm, NewNodePnpmAnalyzer())
}

func NewNodePnpmAnalyzer() *pnpmAnalyzer {
	return &pnpmAnalyzer{}
}

func (a pnpmAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case pnpmLockStatus:
		pkgs, err := ParseLanguageConfiguration(fi, pnpm.NewParser())
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a pnpmAnalyzer) Match(info MatchInfo) int {

	if info.fi.Name() == pnpmLockYaml {
		return pnpmLockStatus
	}

	return 0
}
