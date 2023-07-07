package analyzer

import (
	"path"

	"github.com/aquasecurity/go-dep-parser/pkg/golang/mod"
	"github.com/aquasecurity/go-dep-parser/pkg/golang/sum"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/types"
	"github.com/yaklang/yaklang/common/utils"
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

func (a goModAnalyzer) Analyze(afi AnalyzeFileInfo) ([]types.Package, error) {
	fi := afi.self
	switch fi.matchStatus {
	case statusGoMod:
		p := mod.NewParser(true)
		pkgs, err := ParseLanguageConfiguration(fi, p)
		if err != nil {
			return nil, utils.Errorf("go mod parse error: %s", err)
		}
		// if golang version < 1.17, need to parse go.sum
		if lessThanGo117(pkgs) {
			sumPath := path.Join(path.Dir(fi.path), goSumFile)
			if sfi, ok := afi.matchedFileInfos[sumPath]; ok {
				sp := sum.NewParser()
				sumPkgs, err := ParseLanguageConfiguration(sfi, sp)
				if err != nil {
					return nil, utils.Errorf("go sum parse error: %s", err)
				}
				_, subPkgs := lo.Difference(pkgs, sumPkgs)
				subPkgs = lo.Map(subPkgs, func(item types.Package, index int) types.Package {
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

func lessThanGo117(pkgs []types.Package) bool {
	for _, pkg := range pkgs {
		// The indirect field is populated only in Go 1.17+
		if pkg.Indirect {
			return false
		}
	}
	return true
}
