package analyzer

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/aquasecurity/go-dep-parser/pkg/java/pom"
)

const (
	TypPom TypAnalyzer = "pom-lang"

	MavenPom = "pom.xml"

	statusPom int = 1
)

func init() {
	RegisterAnalyzer(TypPom, NewJavaPomAnalyzer())
}

type pomAnalyzer struct{}

func NewJavaPomAnalyzer() *pomAnalyzer {
	return &pomAnalyzer{}
}

func (a pomAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusPom:
		p := pom.NewParser(fi.Path, pom.WithOffline(true))
		pkgs, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, err
		}
		return pkgs, nil
	}
	return nil, nil
}

func (a pomAnalyzer) Match(info MatchInfo) int {
	if filepath.Base(info.path) == MavenPom {
		return statusPom
	}
	return 0
}
