package ssaapi

import (
	"regexp"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var _ sfvm.ValueOperator = new(Values)

func (p Values) GetOpcode() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}

func (value Values) GetCalled() (sfvm.ValueOperator, error) {
	var vv Values
	for _, i := range value {
		i, err := i.GetCalled()
		if err != nil {
			continue
		}
		if vs, ok := i.(Values); ok {
			vv = append(vv, vs...)
		} else if v, ok := i.(*Value); ok {
			vv = append(vv, v)
		}
	}
	return vv, nil
}

func (Values) GetBinaryOperator() string {
	return ""
}

func (Values) GetUnaryOperator() string {
	return ""
}

func (value Values) GetFields() (sfvm.ValueOperator, error) {
	var vv []sfvm.ValueOperator
	for _, i := range value {
		i, err := i.GetFields()
		if err != nil {
			continue
		}
		_ = i.Recursive(func(operator sfvm.ValueOperator) error {
			if _, ok := operator.(*Value); ok {
				vv = append(vv, operator)
			}
			return nil
		})
	}
	return sfvm.NewValues(vv), nil
}

func (value Values) Recursive(f func(operator sfvm.ValueOperator) error) error {
	for _, v := range value {
		err := f(v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (value Values) IsMap() bool {
	return false
}

func (value Values) IsList() bool {
	return true
}

func (value Values) Len() int {
	return len(value)
}

func (values Values) ExactMatch(mod int, want string) (bool, sfvm.ValueOperator, error) {
	// log.Infof("ExactMatch: %v %v", mod, want)
	newValue := _SearchValues(values, mod, func(s string) bool { return s == want }, sfvm.WithAnalysisContext_Label("search-exact:"+want))
	return len(newValue) > 0, newValue, nil
}

func (values Values) GlobMatch(mod int, g string) (bool, sfvm.ValueOperator, error) {
	newValue := _SearchValues(values, mod, func(s string) bool {
		return glob.MustCompile(g).Match(s)
	}, sfvm.WithAnalysisContext_Label("search-glob:"+g))
	return len(newValue) > 0, newValue, nil
}

func (values Values) RegexpMatch(mod int, re string) (bool, sfvm.ValueOperator, error) {
	newValue := _SearchValues(values, mod, func(s string) bool {
		return regexp.MustCompile(re).MatchString(s)
	}, sfvm.WithAnalysisContext_Label("search-regexp:"+re))
	return len(newValue) > 0, newValue, nil
}

func (value Values) GetCallActualParams(index int) (sfvm.ValueOperator, error) {
	var ret Values
	for _, i := range value {
		vs, err := i.GetCallActualParams(index)
		if err != nil {
			continue
		}
		ret = append(ret, vs.(Values)...)
	}
	if len(ret) == 0 {
		return nil, utils.Errorf("ssa.Values no this argument by index %d", index)
	} else {
		return ret, nil
	}
}

func (value Values) GetAllCallActualParams() (sfvm.ValueOperator, error) {
	var ret Values
	for _, i := range value {
		vs, err := i.GetAllCallActualParams()
		if err != nil {
			continue
		}
		ret = append(ret, vs.(Values)...)
	}
	return ret, nil
}

func (value Values) GetMembersByString(key string) (sfvm.ValueOperator, error) {
	var vals Values
	for _, v := range value {
		if !v.IsObject() {
			continue
		}
		if v.IsMap() || v.IsList() || v.IsObject() {
			res := v.GetMember(v.NewValue(ssa.NewConst(key)))
			vals = append(vals, res)
		}
	}
	return vals, nil
}

func (value Values) GetSyntaxFlowUse() (sfvm.ValueOperator, error) {
	return value.GetUsers(), nil
}
func (value Values) GetSyntaxFlowDef() (sfvm.ValueOperator, error) {
	return value.GetOperands(), nil
}
func (value Values) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return WithSyntaxFlowConfig(sfResult, sfConfig, value.GetTopDefs, config...), nil
}

func (value Values) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return WithSyntaxFlowConfig(sfResult, sfConfig, value.GetBottomUses, config...), nil
}

func (value Values) ListIndex(i int) (sfvm.ValueOperator, error) {
	if i < 0 || i >= len(value) {
		return nil, utils.Error("index out of range")
	}
	return value[i], nil
}

func (value Values) Merge(values ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	var results []sfvm.ValueOperator
	if value != nil {
		results = append(results, value)
	}
	results = append(results, values...)
	return sfvm.NewValues(results), nil
}

func (value Values) Remove(values ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	var results = make(map[int64]sfvm.ValueOperator)
	for _, v := range value {
		results[v.GetId()] = v
	}
	sfvm.NewValues(values).Recursive(func(v sfvm.ValueOperator) error {
		if raw, ok := v.(ssa.GetIdIF); ok {
			delete(results, raw.GetId())
		}
		return nil
	})
	var ret []sfvm.ValueOperator
	for _, v := range results {
		ret = append(ret, v)
	}
	return sfvm.NewValues(ret), nil
}

func (value Values) AppendPredecessor(operator sfvm.ValueOperator, opts ...sfvm.AnalysisContextOption) error {
	for _, element := range value {
		err := element.AppendPredecessor(operator, opts...)
		if err != nil {
			log.Warnf("append predecessor failed: %v", err)
		}
	}
	return nil
}

func (value Values) FileFilter(string, string, map[string]string, []string) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Values is not supported file filter")
}
