package binary

import (
	"debug/buildinfo"
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

var (
	ErrUnrecognizedExe = errors.New("unrecognized executable format")
	ErrNonGoBinary     = errors.New("non go binary")
)

// convertError detects buildinfo.errUnrecognizedFormat and convert to
// ErrUnrecognizedExe and convert buildinfo.errNotGoExe to ErrNonGoBinary
func convertError(err error) error {
	errText := err.Error()
	if strings.HasSuffix(errText, "unrecognized file format") {
		return ErrUnrecognizedExe
	}
	if strings.HasSuffix(errText, "not a Go executable") {
		return ErrNonGoBinary
	}

	return err
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

// Parse reads debug/buildinfo. It does not invent licenses or source lines;
// those fields are not present in Go buildinfo. Main (devel) is the binary
// identity, not a dependency, and is not listed as a component.
func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	st := budget.From(types.ContextOf(r))
	info, err := buildinfo.Read(r)
	if err != nil {
		return nil, nil, convertError(err)
	}
	if err := st.Result(budget.SizeSlice); err != nil {
		return nil, nil, err
	}
	var libs []types.Library
	for _, dep := range info.Deps {
		if dep.Path == "" {
			continue
		}
		mod := dep
		if dep.Replace != nil {
			mod = dep.Replace
			if mod.Path == "" {
				continue
			}
		}
		if err := st.Result(budget.SizeOfPackage(mod.Path, mod.Version, mod.Sum)); err != nil {
			return nil, nil, err
		}
		lib := types.Library{
			Name:            mod.Path,
			Version:         mod.Version,
			Verification:    mod.Sum,
			DeclaredName:    dep.Path,
			DeclaredVersion: dep.Version,
			Evidence:        "binary",
			ID:              dep.Path,
		}
		if dep.Replace != nil {
			lib.Source = dep.Path
		}
		libs = append(libs, lib)
	}
	return libs, nil, nil
}
