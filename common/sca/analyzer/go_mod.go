package analyzer

import (
	"errors"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/gomod"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"io/fs"
	"path"
	"strings"
)

const (
	TypGoMod TypAnalyzer = "go-mod-lang"

	goModFile = "go.mod"
	goSumFile = "go.sum"

	statusGoMod int = 1
	statusGoSum int = 2
)

var goModRequiredFiles = []string{
	"go.mod",
	"go.sum",
}

func init() {
	RegisterAnalyzer(TypGoMod, NewGoModAnalyzer())
}

type goModAnalyzer struct{}

func NewGoModAnalyzer() *goModAnalyzer {
	return &goModAnalyzer{}
}

func (a goModAnalyzer) Match(info MatchInfo) int {
	_, fileName := info.fileSystem.PathSplit(info.Path)
	if fileName == goModFile {
		return statusGoMod
	} else if fileName == goSumFile {
		return statusGoSum
	}
	return 0
}

// Analyze reads declarations only. A checksum in go.sum is not evidence of use.
func (a goModAnalyzer) Analyze(afi AnalyzeFileInfo) ([]*dxtypes.Package, error) {

	const maxBytes = 16 << 20
	data, err := textdecode.ReadRaw(afi.Self.LazyFile.Context(), afi.Self.LazyFile, maxBytes)
	if err != nil {
		return nil, err
	}
	if afi.Self.MatchStatus == statusGoSum {
		_, err := gomod.ParseSum(afi.Self.LazyFile.Context(), data)
		return nil, err
	}
	l := budget.From(afi.Self.LazyFile.Context()).Limits
	f, err := gomod.Parse(afi.Self.LazyFile.Context(), data, gomod.Limits{MaxBytes: min(maxBytes, int(l.MaxFileBytes)), MaxTokenBytes: l.MaxFieldBytes, MaxStatements: l.MaxExpressionNodes})
	if err != nil {
		return nil, err
	}
	var sums map[gomod.SumKey]string
	if afi.Self.filesystem != nil {
		sumName := path.Join(path.Dir(afi.Self.Path), "go.sum")
		raw, e := afi.Self.filesystem.ReadFile(sumName)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return nil, e
		}
		if e == nil {
			sums, e = gomod.ParseSum(afi.Self.LazyFile.Context(), raw)
			if e != nil {
				return nil, e
			}
		}
	}
	declarations := f.Declarations()
	pkgs := make([]*dxtypes.Package, 0, len(declarations))
	for _, d := range declarations {
		p := &dxtypes.Package{Name: d.Effective.Path, Version: strings.TrimPrefix(d.Effective.Version, "v"), PackageDetails: &dxtypes.PackageDetails{Ecosystem: "golang", Evidence: "declared", DeclaredName: d.Requirement.Path, DeclaredVersion: d.Requirement.Version, Indirect: d.Requirement.Indirect}}
		if d.Replacement != nil {
			p.Source = d.Replacement.New.Path
			p.ReplacementVersion = d.Replacement.New.Version
		}
		p.Verification = sums[gomod.SumKey{Path: d.Effective.Path, Version: d.Effective.Version}]
		p.StartLine, p.EndLine = d.Requirement.Line, d.Requirement.Line
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}
