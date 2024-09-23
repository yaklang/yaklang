package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

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
func (p *Program) SyntaxFlowWithRule(rule *schema.SyntaxFlowRule, opts ...sfvm.Option) (*SyntaxFlowResult, error) {
	return SyntaxFlowWithDb(p, rule, opts...)
}

func (ps Programs) SyntaxFlowWithError(i string, opts ...sfvm.Option) (*SyntaxFlowResult, error) {
	return SyntaxFlowWithError(
		sfvm.NewValues(lo.Map(ps, func(p *Program, _ int) sfvm.ValueOperator { return p })),
		i, opts...,
	)
}
func (ps Programs) SyntaxFlowWithRule(rule *schema.SyntaxFlowRule, opts ...sfvm.Option) (*SyntaxFlowResult, error) {
	return SyntaxFlowWithDb(
		sfvm.NewValues(lo.Map(ps, func(p *Program, _ int) sfvm.ValueOperator { return p })),
		rule, opts...,
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
	return CreateResultFromQuery(res), err
}
func SyntaxFlowWithDb(p sfvm.ValueOperator, rule *schema.SyntaxFlowRule, opts ...sfvm.Option) (*SyntaxFlowResult, error) {
	if utils.IsNil(p) {
		return nil, utils.Errorf("SyntaxFlowWithError: base ValueOperator is nil")
	}
	vm := sfvm.NewSyntaxFlowVirtualMachine(opts...)
	frame, err := vm.CompileFromDb(rule)
	if err != nil {
		return nil, utils.Errorf("SyntaxFlow compile %#v failed: %v", rule.OpCodes, err)
	}
	feed, err := frame.Feed(p)
	return CreateResultFromQuery(feed), err
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
	//暂时未启用，后续如果config需要使用外部变量可以启用 context
	frame.WithContext(sfResult)
	res, err := frame.Feed(p)
	return CreateResultFromQuery(res), err
}
