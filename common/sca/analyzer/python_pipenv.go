package analyzer

import (
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/aquasecurity/go-dep-parser/pkg/python/pipenv"
)

const (
	TypPythonPIPEnv TypAnalyzer = "python-pipenv-lang"

	pipenvLockFile = "pipfile.lock"

	statusPIPenvLock int = 1
)

func init() {
	RegisterAnalyzer(TypPythonPIPEnv, NewPythonPIPEnvAnalyzer())
}

type pythonPIPEnvAnalyzer struct{}

func NewPythonPIPEnvAnalyzer() *pythonPIPEnvAnalyzer {
	return &pythonPIPEnvAnalyzer{}
}

func (a pythonPIPEnvAnalyzer) Match(info MatchInfo) int {
	if strings.HasSuffix(strings.ToLower(info.path), pipFile) {
		return statusPIPenvLock
	}
	return 0
}

func (a pythonPIPEnvAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self

	switch fi.MatchStatus {
	case statusPIPenvLock:
		return ParseLanguageConfiguration(fi, pipenv.NewParser())
	}

	return nil, nil
}
