package ssa

type InspectVariableResult struct {
	VariableName   string
	ProbablyTypes  []string
	ProbablyValues []string
}

func (r *InspectVariableResult) Merge(other *InspectVariableResult) {
	if r.VariableName != other.VariableName {
		return
	}
	r.ProbablyTypes = append(r.ProbablyTypes, other.ProbablyTypes...)
	r.ProbablyValues = append(r.ProbablyValues, other.ProbablyValues...)
}

func (p *Program) InspectVariable(varName string) *InspectVariableResult {
	var result = new(InspectVariableResult)
	result.VariableName = varName

	for _, pkg := range p.Packages {
		for _, funcIns := range pkg.funcs {
			result.Merge(funcIns.InspectVariable(varName))
		}
	}
	return result
}

func (f *Function) InspectVariable(varName string) *InspectVariableResult {
	var result = new(InspectVariableResult)
	result.VariableName = varName

	if f == nil || f.currentDef == nil {
		return result
	}

	res, ok := f.currentDef[varName]
	if !ok || res == nil {
		return result
	}
	var probablyTypes []string
	for block, value := range res {
		_ = block
		valType := value.GetType()
		if valType != nil {
			probablyTypes = append(probablyTypes, valType.String())
		}
	}
	result.ProbablyTypes = probablyTypes
	return result
}
