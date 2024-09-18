package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
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
			if valueLen := sfvm.ValuesLen(value); valueLen > 0 {
				r.variable.Set(name, valueLen)
			}
			return true
		})
		for name := range r.memResult.AlertSymbolTable {
			if v, ok := r.variable.Get(name); ok && v.(int) > 0 {
				r.alertVariable = append(r.alertVariable, name)
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
	}
	return r.unName
}
