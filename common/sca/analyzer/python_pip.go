package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"strings"

	"github.com/aquasecurity/go-dep-parser/pkg/python/pip"
)

const (
	TypPythonPIP TypAnalyzer = "python-pip-lang"

	pipFile = "requirements.txt"

	statusPIP int = 1
)

func init() {
	RegisterAnalyzer(TypPythonPIP, NewPythonPIPAnalyzer())
}

type pythonPIPAnalyzer struct{}

func NewPythonPIPAnalyzer() *pythonPIPAnalyzer {
	return &pythonPIPAnalyzer{}
}

func (a pythonPIPAnalyzer) Match(info MatchInfo) int {
	if strings.HasSuffix(info.path, pipFile) {
		return statusPIP
	}
	return 0
}

func (a pythonPIPAnalyzer) Analyze(afi AnalyzeFileInfo) ([]dxtypes.Package, error) {
	fi := afi.Self

	switch fi.MatchStatus {
	case statusPIP:
		return ParseLanguageConfiguration(fi, pip.NewParser())
	}

	return nil, nil
}
