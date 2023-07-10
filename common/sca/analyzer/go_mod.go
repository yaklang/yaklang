package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"path"

	"github.com/aquasecurity/go-dep-parser/pkg/golang/mod"
	"github.com/aquasecurity/go-dep-parser/pkg/golang/sum"
	"github.com/samber/lo"
)

const (
	TypGoMod TypAnalyzer = "go-mod-lang"

	goModFile = "go.mod"
	goSumFile = "go.sum"

	statusGoMod int = 1
	statusGoSum int = 2
)

var (
	goModRequiredFiles = []string{
		"go.mod",
		"go.sum",
	}
)

func init() {
	RegisterAnalyzer(TypGoMod, NewGoModAnalyzer())
}

type goModAnalyzer struct{}

func NewGoModAnalyzer() *goModAnalyzer {
	return &goModAnalyzer{}
}

func (a goModAnalyzer) Match(info MatchInfo) int {
	fileName := path.Base(info.path)
	if fileName == goModFile {
		return statusGoMod
	} else if fileName == goSumFile {
		return statusGoSum
	}
	return 0
}

func (a goModAnalyzer) Analyze(afi AnalyzeFileInfo) ([]dxtypes.Package, error) {
	fi := afi.Self
	switch fi.MatchStatus {
	case statusGoMod:
		p := mod.NewParser(true)
		pkgs, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, err
		}
		// if golang version < 1.17, need to parse go.sum
		if lessThanGo117(pkgs) {
			sumPath := path.Join(path.Dir(fi.Path), goSumFile)
			if sfi, ok := afi.MatchedFileInfos[sumPath]; ok {
				sp := sum.NewParser()
				sumPkgs, err := ParseLanguageConfiguration(sfi, sp)
				if err != nil {
					return nil, err
				}
				var originalPkg = make(map[string]dxtypes.Package, len(pkgs))
				for _, pkg := range pkgs {
					originalPkg[pkg.Identifier()] = pkg
				}
				var subPkgs []dxtypes.Package
				for _, sPkg := range sumPkgs {
					_, ok := originalPkg[sPkg.Identifier()]
					if ok {
						continue
					}
					subPkgs = append(subPkgs, sPkg)
				}
				subPkgs = lo.Map(subPkgs, func(item dxtypes.Package, index int) dxtypes.Package {
					item.Indirect = true
					return item
				})
				pkgs = append(pkgs, subPkgs...)
			}
		}
		return pkgs, nil
	}
	return nil, nil
}

func lessThanGo117(pkgs []dxtypes.Package) bool {
	for _, pkg := range pkgs {
		// The indirect field is populated only in Go 1.17+
		if pkg.Indirect {
			return false
		}
	}
	return true
}
