package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

type SyntaxFlowResult struct {
	*sfvm.SFFrameResult
	symbol map[string]Values
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

func SyntaxFlowWithOldEnv(p sfvm.ValueOperator, sfCode string, sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config) (*SyntaxFlowResult, error) {
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
