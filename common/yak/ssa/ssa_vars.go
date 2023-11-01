package ssa

import (
	"github.com/samber/lo"
)

type InspectVariableResult struct {
	VariableName   string
	ProbablyTypes  Types
	ProbablyValues Values
}

func (r *InspectVariableResult) Merge(other *InspectVariableResult) {
	if r.VariableName != other.VariableName {
		return
	}
	r.ProbablyTypes = append(r.ProbablyTypes, other.ProbablyTypes...)
	r.ProbablyValues = append(r.ProbablyValues, other.ProbablyValues...)
}

func (p *Program) InspectVariableLast(varName string) *InspectVariableResult {
	var result = new(InspectVariableResult)
	for _, pkg := range p.Packages {
		for _, funcIns := range pkg.Funcs {
			if res, ok := funcIns.symbolTable[varName]; ok {
				vs := res[funcIns.ExitBlock]
				v := vs[len(vs)-1]
				result.ProbablyValues = append(result.ProbablyValues, v)
				result.ProbablyTypes = append(result.ProbablyTypes, v.GetType())
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
	var probablyTypes Types
	var probablyValue Values
	for _, v := range res {
		probablyValue = append(probablyValue, v...)
		probablyTypes = append(probablyTypes, lo.Map(v, func(v Value, _ int) Type { return v.GetType() })...)
	}
	result.ProbablyTypes = probablyTypes
	result.ProbablyValues = probablyValue
	return result
}

func (f *Function) GetValuesByName(name string) []Node {
	ret := make([]Node, 0)
	if table, ok := f.symbolTable[name]; ok {
		for _, v := range table {
			for _, v := range v {
				ret = append(ret, v)
			}
		}
	}
	return lo.Uniq(ret)
}
