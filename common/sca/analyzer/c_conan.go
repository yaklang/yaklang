package analyzer

import (
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/aquasecurity/go-dep-parser/pkg/c/conan"
)

const (
	TypClangConan TypAnalyzer = "conan-lang"

	ConanLock = "conan.lock"

	statusConan int = 1
)

func init() {
	RegisterAnalyzer(TypClangConan, NewConanAnalyzer())
}

type conanAnalyzer struct{}

func NewConanAnalyzer() *conanAnalyzer {
	return &conanAnalyzer{}
}

func (a conanAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusConan:
		p := conan.NewParser()
		res, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, err
		}
		return res, nil
	}
	return nil, nil
}

func (a conanAnalyzer) Match(info MatchInfo) int {
	if strings.HasSuffix(info.path, ConanLock) {
		return statusConan
	}
	return 0
}
