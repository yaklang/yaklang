package ssa4analyze

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// program pass
type Analyzer interface {
	Run(*ssa.Program)
}
type AnalyzerBuilder func(config) Analyzer

var (
	analyzerBuilders = []AnalyzerBuilder{
		NewBlockCondition,
		NewTypeInference,
		NewTypeCheck,
	}
)

// analyzer group
type AnalyzerGroup struct {
	Ir     *ssa.Program
	config config
}

func (ag *AnalyzerGroup) GetError() ssa.SSAErrors {
	return ag.Ir.GetErrors()
}

func (ag *AnalyzerGroup) Run() {
	if ag.Ir == nil {
		return
	}
	for _, builder := range analyzerBuilders {
		builder(ag.config).Run(ag.Ir)
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
func RunAnalyzer(prog *ssa.Program, opts ...Option) {
	NewAnalyzerGroup(prog, opts...).Run()
}
