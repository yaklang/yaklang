package ssa4analyze

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze/pass"
)

// analyzer
type Analyzer interface {
	Analyze(config, *ssa.Program)
}

var (
	analyzers = make([]Analyzer, 0)
)

func RegisterAnalyzer(a Analyzer) {
	analyzers = append(analyzers, a)
}

// analyzer group
type AnalyzerGroup struct {
	Ir     *ssa.Program
	config config
}

func (ag *AnalyzerGroup) GetError() ssa.SSAErrors {
	return ag.Ir.GetErrors()
}

func (ag *AnalyzerGroup) Run() {
	if ag.config.enablePass {
		for _, pass := range pass.GetPass() {
			pass.Run(ag.Ir)
		}
	}
	for _, a := range ag.config.analyzers {
		a.Analyze(ag.config, ag.Ir)
	}
}

func NewAnalyzerGroup(prog *ssa.Program, opts ...Option) *AnalyzerGroup {
	config := defaultConfig()
	for _, opt := range opts {
		opt(&config)
	}

	return &AnalyzerGroup{
		Ir:     prog,
		config: config,
	}
}
