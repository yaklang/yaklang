package ssaapi

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

type SyntaxFlowResult struct {
	*sfvm.SFFrameResult
	symbol map[string]Values
}

type showConfig struct {
	showCode     bool
	showDot      bool
	lessVariable bool
	showAll      bool
}

type ShowHandle func(config *showConfig)

func WithShowAll(show bool) ShowHandle {
	return func(config *showConfig) {
		config.showAll = show
	}
}
func WithShowCode(show bool) ShowHandle {
	return func(config *showConfig) {
		config.showCode = show
	}
}
func WithShowDot(show bool) ShowHandle {
	return func(config *showConfig) {
		config.showDot = show
	}
}
func WithLessVariable(show bool) ShowHandle {
	return func(config *showConfig) {
		config.lessVariable = show
	}
}
func (s *SyntaxFlowResult) Show(handle ...ShowHandle) {
	var (
		_config = new(showConfig)
	)

	for _, f := range handle {
		f(_config)
	}

	fmt.Println(s.StringEx(_config.showAll))
	if _config.showAll {
		s.GetAllValues()
	} else {
		if len(s.AlertSymbolTable) > 0 {
			for name, _ := range s.AlertSymbolTable {
				s.GetValues(name)
			}
		} else {
			s.GetValues("_")
		}
	}
	lo.ForEach(lo.Entries(s.symbol), func(item lo.Entry[string, Values], index int) {
		if _config.showCode {
			log.Infof("===================== Variable:%v =================== ", item.Key)
			for _, value := range item.Value {
				value.ShowWithSourceCode()
			}
		}
		if _config.showDot {
			log.Infof("===================== DOT =================== ")
			item.Value.ShowDot()
		}
	})
}

func (r *SyntaxFlowResult) GetValues(name string) Values {
	if r == nil || r.symbol == nil || r.SFFrameResult == nil {
		return nil
	}
	if vs, ok := r.symbol[name]; ok {
		return vs
	}
	if v, ok := r.SFFrameResult.SymbolTable.Get(name); ok {
		vs := SyntaxFlowVariableToValues(v)
		r.symbol[name] = vs
		return vs
	}
	return nil
}

func (r *SyntaxFlowResult) GetAllValues() map[string]Values {
	if r == nil || r.symbol == nil || r.SFFrameResult == nil {
		return nil
	}
	for name := range r.SFFrameResult.SymbolTable.GetMap() {
		r.GetValues(name)
	}
	return r.symbol
}

func (r *SyntaxFlowResult) GetAllValuesChain() Values {
	var results Values
	m := r.GetAllValues()
	for name, vs := range m {
		if name == "_" {
			continue
		}
		results = append(results, vs...)
	}
	if len(results) == 0 {
		results = append(results, r.GetValues("_")...)
	}
	return results
}

func (p *Program) SyntaxFlow(i string, opts ...sfvm.Option) *SyntaxFlowResult {
	res, err := p.SyntaxFlowWithError(i, opts...)
	if err != nil {
		log.Warnf("exec syntaxflow: %#v failed: %v", i, err)
	}
	return res
}

func (p *Program) SyntaxFlowChain(i string, opts ...sfvm.Option) Values {
	var results Values
	res, err := p.SyntaxFlowWithError(i, opts...)
	if err != nil {
		log.Warnf("syntax_flow_chain_failed: %s", err)
	}
	if res == nil {
		return results
	}
	return res.GetAllValuesChain()
}

func (p *Program) SyntaxFlowWithError(i string, opts ...sfvm.Option) (*SyntaxFlowResult, error) {
	return SyntaxFlowWithError(p, i, opts...)
}

func (ps Programs) SyntaxFlowWithError(i string, opts ...sfvm.Option) (*SyntaxFlowResult, error) {
	return SyntaxFlowWithError(
		sfvm.NewValues(lo.Map(ps, func(p *Program, _ int) sfvm.ValueOperator { return p })),
		i, opts...,
	)
}

func SyntaxFlowWithError(p sfvm.ValueOperator, sfCode string, opts ...sfvm.Option) (*SyntaxFlowResult, error) {
	if utils.IsNil(p) {
		return nil, utils.Errorf("SyntaxFlowWithError: base ValueOperator is nil")
	}
	vm := sfvm.NewSyntaxFlowVirtualMachine(opts...)
	frame, err := vm.Compile(sfCode)
	if err != nil {
		return nil, utils.Errorf("SyntaxFlow compile %#v failed: %v", sfCode, err)
	}
	res, err := frame.Feed(p)
	return &SyntaxFlowResult{
		SFFrameResult: res,
		symbol:        make(map[string]Values),
	}, err
}

func SyntaxFlowWithVMContext(p sfvm.ValueOperator, sfCode string, sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config) (*SyntaxFlowResult, error) {
	if utils.IsNil(p) {
		return nil, utils.Errorf("SyntaxFlowWithError: base ValueOperator is nil")
	}
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	vm.SetConfig(sfConfig)
	frame, err := vm.Compile(sfCode)
	if err != nil {
		return nil, utils.Errorf("SyntaxFlow compile %#v failed: %v", sfCode, err)
	}
	frame.SetSFResult(sfResult)
	res, err := frame.Feed(p)
	return &SyntaxFlowResult{
		SFFrameResult: res,
		symbol:        make(map[string]Values),
	}, err
}
