package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

var _ sfvm.ValueOperator = &Program{}

func (p *Program) GetName() string {
	return p.Program.GetProgramName()
}

func (p *Program) IsMap() bool { return false }

func (p *Program) IsList() bool {
	//TODO implement me
	return false
}

func (p *Program) ExactMatch(s string) (bool, sfvm.ValueOperator, error) {
	values := p.Ref(s)
	return len(values) > 0, values, nil
}

func (p *Program) GlobMatch(g sfvm.Glob) (bool, sfvm.ValueOperator, error) {
	values := p.GlobRef(g)
	return len(values) > 0, values, nil
}

func (p *Program) RegexpMatch(re *regexp.Regexp) (bool, sfvm.ValueOperator, error) {
	values := p.RegexpRef(re)
	return len(values) > 0, values, nil
}

func (p *Program) GetMembers() (sfvm.ValueOperator, error) {
	return p.GlobRefRaw("*").Flat(func(value *Value) Values {
		if value.IsObject() {
			return value.GetAllMember()
		}
		return nil
	}), nil
}

func (p *Program) ListIndex(i int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported list index")
}

func (p *Program) GetCallActualParams() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported call actual params")
}

func (p *Program) GetSyntaxFlowTopDef() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow top def")
}

func (p *Program) GetSyntaxFlowBottomUse() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow bottom use")
}

func (p *Program) GetCalled() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported called")
}

func (p *Program) SyntaxFlow(i string, opts ...any) Values {
	vals, err := p.SyntaxFlowWithError(i)
	if err != nil {
		log.Warnf("exec syntaxflow: %#v failed: %v", i, err)
		return nil
	}
	return vals
}

func (p *Program) SF(i string, opts ...any) Values {
	return p.SyntaxFlow(i, opts...)
}

func (p *Program) SyntaxFlowWithError(i string, opts ...any) (Values, error) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	vm.Debug()
	err := vm.Compile(i)
	if err != nil {
		return nil, utils.Errorf("SyntaxFlow compile %#v failed: %v", i, err)
	}
	results := vm.Feed(p)
	if err != nil {
		return nil, utils.Errorf("SyntaxFlow feed %#v failed: %v", i, err)
	}

	var vals []*Value
	for _, v := range results.Values() {
		switch ret := v.(type) {
		case *Value:
			vals = append(vals, ret)
		case Values:
			vals = append(vals, ret...)
		case *sfvm.ValueList:
			values, err := SFValueListToValues(ret)
			if err != nil {
				log.Warnf("cannot handle type: %T error: %v", v, err)
			}
			vals = append(vals, values...)
		default:
			log.Warnf("cannot handle type(raw): %T", i)
		}
	}
	return vals, nil
}

func SFValueListToValues(list *sfvm.ValueList) (Values, error) {
	return _SFValueListToValues(0, list)
}

func _SFValueListToValues(count int, list *sfvm.ValueList) (Values, error) {
	if count > 1000 {
		return nil, utils.Errorf("too many nested ValueList: %d", count)
	}
	var vals Values
	list.ForEach(func(i any) {
		switch element := i.(type) {
		case *Value:
			vals = append(vals, element)
		case Values:
			vals = append(vals, element...)
		case *sfvm.ValueList:
			ret, err := _SFValueListToValues(count+1, element)
			if err != nil {
				log.Warnf("cannot handle type: %T error: %v", i, err)
			} else {
				vals = append(vals, ret...)
			}
		default:
			log.Warnf("cannot handle type: %T", i)
		}
	})
	return vals, nil
}
