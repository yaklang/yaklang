package ssa

type InspectVariableResult struct {
	VariableName   string
	ProbablyTypes  []string
	ProbablyValues Values
	MustTypes      []string
	MustValue      Values
}

func (r *InspectVariableResult) Merge(other *InspectVariableResult) {
	if r.VariableName != other.VariableName {
		return
	}
	r.ProbablyTypes = append(r.ProbablyTypes, other.ProbablyTypes...)
	r.ProbablyValues = append(r.ProbablyValues, other.ProbablyValues...)
	r.MustTypes = append(r.MustTypes, other.MustTypes...)
	r.MustValue = append(r.MustValue, other.MustValue...)
}

func (p *Program) InspectVariableLast(varName string) *InspectVariableResult {
	var result = new(InspectVariableResult)
	for _, pkg := range p.Packages {
		for _, funcIns := range pkg.Funcs {
			if res, ok := funcIns.symbolTable[varName]; ok {
				_ = res
				// last := res[len(res)-1]
				// result.ProbablyTypes = append(result.ProbablyTypes, last.GetType().String())
				// result.ProbablyValues = append(result.ProbablyValues, last)
			}
		}
	}
	result.VariableName = varName
	return result
}

func (p *Program) InspectVariable(varName string) *InspectVariableResult {
	var result = new(InspectVariableResult)
	result.VariableName = varName

	for _, pkg := range p.Packages {
		for _, funcIns := range pkg.Funcs {
			result.Merge(funcIns.InspectVariable(varName))
		}
	}
	return result
}

func (f *Function) InspectVariable(varName string) *InspectVariableResult {
	var result = new(InspectVariableResult)
	result.VariableName = varName

	if f == nil || f.symbolTable == nil {
		return result
	}

	res, ok := f.symbolTable[varName]
	if !ok || res == nil {
		return result
	}
	var probablyTypes []string
	var probablyValue Values
	var mustValue Values
	var mustTypes []string
	values := make([]Value, 0)
	for _, v := range res {
		_ = v
		// values = append(values, v)
		// probablyValue = append(probablyValue, v)
		// probablyTypes = append(probablyTypes, v.GetType().String())
		// if inst, ok := v.(Instruction); ok {
		// 	reachable := inst.GetBlock().Reachable()
		// 	if reachable == 1 {
		// 		mustTypes = append(mustTypes, v.GetType().String())
		// 		mustValue = append(mustValue, v)
		// 	}
		// }
	}
	result.ProbablyTypes = probablyTypes
	result.ProbablyValues = probablyValue
	result.MustTypes = mustTypes
	result.MustValue = mustValue
	_ = values
	// result.values = values
	return result
}
