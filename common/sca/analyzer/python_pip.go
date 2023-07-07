package analyzer

import (
	"strings"

	"github.com/aquasecurity/go-dep-parser/pkg/python/pip"
	"github.com/yaklang/yaklang/common/sca/types"
)

const (
	TypPythonPIP TypAnalyzer = "python-pip-lang"

	pipFile = "requirements.txt"

	statusPIP int = 1
)

func init() {
	RegisterAnalyzer(TypPythonPackaging, NewPythonPackagingAnalyzer())
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

func (a pythonPIPAnalyzer) Analyze(afi AnalyzeFileInfo) ([]types.Package, error) {
	fi := afi.self

	switch fi.matchStatus {
	case statusPIP:
		return ParseLanguageConfiguration(fi, pip.NewParser())
	}

	return nil, nil
}
