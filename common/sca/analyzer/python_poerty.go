package analyzer

import (
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/aquasecurity/go-dep-parser/pkg/python/poetry"
)

const (
	TypPythonPoetry TypAnalyzer = "python-poetry-lang"

	PoetryLockFile = "poetry.lock"
	PyProjectFile  = "pyproject.toml"

	statusPoetry int = 1
)

func init() {
	RegisterAnalyzer(TypPythonPoetry, NewPythonPoetryAnalyzer())
}

type pythonPoetryAnalyzer struct{}

func NewPythonPoetryAnalyzer() *pythonPoetryAnalyzer {
	return &pythonPoetryAnalyzer{}
}

func (a pythonPoetryAnalyzer) Match(info MatchInfo) int {
	if strings.HasSuffix(info.path, PoetryLockFile) || strings.HasSuffix(info.path, PyProjectFile) {
		return statusPIP
	}
	return 0
}

func (a pythonPoetryAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self

	switch fi.MatchStatus {
	case statusPoetry:
		pkgs, err := ParseLanguageConfiguration(fi, poetry.NewParser())
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}

	return nil, nil
}
