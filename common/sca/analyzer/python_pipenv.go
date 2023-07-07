package analyzer

import (
	"strings"

	"github.com/aquasecurity/go-dep-parser/pkg/python/pipenv"
	"github.com/yaklang/yaklang/common/sca/types"
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

func (a pythonPIPEnvAnalyzer) Analyze(afi AnalyzeFileInfo) ([]types.Package, error) {
	fi := afi.self

	switch fi.matchStatus {
	case statusPIPenvLock:
		return ParseLanguageConfiguration(fi, pipenv.NewParser())
	}

	return nil, nil
}
