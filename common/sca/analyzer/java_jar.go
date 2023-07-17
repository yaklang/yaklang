package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

const (
	TypJar    TypAnalyzer = "jar-lang"
	statusJar int         = 1
)

var (
	jarRequiredExtensions = []string{
		".jar",
		".war",
		".ear",
		".par",
	}
)

func init() {
	RegisterAnalyzer(TypPom, NewJavaJarAnalyzer())
}

type jarAnalyzer struct{}

func NewJavaJarAnalyzer() *jarAnalyzer {
	return &jarAnalyzer{}
}

func (a jarAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusJar:
		fileInfo, err := fi.LazyFile.Stat()
		if err != nil {
			return nil, err
		}
		p := NewJarParser(fi.Path, fileInfo.Size())
		pkgs, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, err
		}

		//

		return pkgs, nil
	}
	return nil, nil
}

func (a jarAnalyzer) Match(info MatchInfo) int {
	ext := filepath.Ext(info.path)
	for _, required := range jarRequiredExtensions {
		if strings.EqualFold(ext, required) {
			return statusJar
		}
	}
	return 0
}
