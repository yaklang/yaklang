package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// ======================================== All Value/Variable ========================================

func (r *SyntaxFlowResult) GetVariableNum() int {
	if r == nil {
		return 0
	}
	if r.variable != nil {
		return r.variable.Len()
	}
	r.GetAllVariable()
	return r.variable.Len()
}

func (r *SyntaxFlowResult) GetAllVariable() *orderedmap.OrderedMap {
	if r == nil {
		return nil
	}
	if r.variable != nil {
		return r.variable
	}

	r.variable = orderedmap.New()
	if r.memResult != nil {
		r.memResult.SymbolTable.ForEach(func(name string, value sfvm.ValueOperator) bool {
			r.variable.Set(name, sfvm.ValuesLen(value))
			return true
		})
		for name := range r.memResult.AlertSymbolTable {
			if v, ok := r.variable.Get(name); ok && v.(int) > 0 {
				r.alertVariable = append(r.alertVariable, name)
			}
		}
	}

	if r.dbResult != nil {
		res, err := ssadb.GetResultVariableByID(ssadb.GetDB(), r.GetResultID())
		if err != nil {
			log.Errorf("err: %v", err)
			return nil
		}
		for _, v := range res {
			if v.Name == "_" {
				continue
			}
			r.variable.Set(v.Name, int(v.ValueNum))
			if v.Alert != "" {
				r.alertVariable = append(r.alertVariable, v.Name)
			}
		}
		for _, name := range r.dbResult.UnValueVariable {
			if _, ok := r.variable.Get(name); !ok {
				r.variable.Set(name, 0)
			}
		}
	}

	return r.variable
}

func (r *SyntaxFlowResult) GetAllValuesChain() Values {
	var results Values
	m := r.GetAllVariable()
	m.ForEach(func(name string, value any) {
		vs := r.GetValues(name)
		results = append(results, vs...)
	})
	if len(results) == 0 {
		results = append(results, r.GetUnNameValues()...)
	}
	return results
}

// ======================================== Single Value ========================================

// Normal value
func (r *SyntaxFlowResult) GetValues(name string) Values {
	if r == nil {
		return nil
	}
	// unname
	if name == "_" {
		return r.GetUnNameValues()
	}
	// cache
	if vs, ok := r.symbol[name]; ok {
		return vs
	}

	// memory
	if r.memResult != nil {
		if v, ok := r.memResult.SymbolTable.Get(name); ok {
			vs := SyntaxFlowVariableToValues(v)
			r.symbol[name] = vs
			return vs
		}
	}
	if r.dbResult != nil {
		vs := r.getValueFromDB(name)
		r.symbol[name] = vs
		return vs
	}
	return nil
}

// Alert value
func (r *SyntaxFlowResult) GetAlertVariables() []string {
	if r == nil {
		return nil
	}
	if r.alertVariable == nil {
		r.GetAllVariable()
	}
	return r.alertVariable
}

// UnName value
func (r *SyntaxFlowResult) GetUnNameValues() Values {
	if r == nil {
		return nil
	}
	if r.unName != nil {
		return r.unName
	}
	if r.memResult != nil {
		// memory
		r.unName = SyntaxFlowVariableToValues(sfvm.NewValues(r.memResult.UnNameValue))
	} else if r.dbResult != nil {
		// database
		r.unName = r.getValueFromDB("_")
	}
	return r.unName
}

func (r *SyntaxFlowResult) GetResultID() uint {
	if r == nil || r.dbResult == nil {
		return 0
	}
	return r.dbResult.ID
}

func (r *SyntaxFlowResult) getValueFromDB(name string) Values {
	resValueID, err := ssadb.GetResultValueByVariable(ssadb.GetDB(), r.GetResultID(), name)
	if err != nil {
		return nil
	}
	vs := lo.Map(resValueID, func(id int64, _ int) *Value {
		return r.newValue(id)
	})
	return vs
}

func (r *SyntaxFlowResult) newValue(valueID int64) *Value {
	node, err := ssa.NewLazyInstruction(valueID)
	if err != nil {
		log.Errorf("GetValues new lazy instruction: %v", err)
		return nil
	}
	return r.program.NewValue(node)
}
