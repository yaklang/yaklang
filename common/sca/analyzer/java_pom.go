package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/java/pom"
)

const (
	TypJavaPom TypAnalyzer = "pom-lang"

	MavenPom = "pom.xml"

	statusPom int = 1
)

func init() {
	RegisterAnalyzer(TypJavaPom, NewJavaPomAnalyzer())
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
	_, filename := info.fileSystem.PathSplit(info.Path)
	if filename == MavenPom {
		return statusPom
	}
	return 0
}
