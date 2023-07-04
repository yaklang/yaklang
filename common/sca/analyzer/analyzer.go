package analyzer

import (
	"io"
	"io/fs"

	"github.com/yaklang/yaklang/common/sca/types"
)

type Analyzer interface {
	Analyze(string, fs.FileInfo, io.Reader) ([]types.Package, error)
}

type AnalyzerGroup struct {
	analyzers []Analyzer
}

func NewAnalyzerGroup() *AnalyzerGroup {
	return &AnalyzerGroup{}
}

func (ag *AnalyzerGroup) Append(a Analyzer) {
	ag.analyzers = append(ag.analyzers, a)
}

func (ag *AnalyzerGroup) Analyze(path string, fi fs.FileInfo, r io.Reader) ([]types.Package, error) {
	pkgs := make([]types.Package, 0)
	for _, a := range ag.analyzers {
		p, err := a.Analyze(path, fi, r)
		if err != nil {
			return nil, err
		}
		if p != nil {
			pkgs = append(pkgs, p...)
		}
	}
	return pkgs, nil
}
