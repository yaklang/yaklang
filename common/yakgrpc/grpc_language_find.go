package yakgrpc

import (
	"context"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func findVariable(v *ssaapi.Value, word string, containPoint bool) []*ssa.Variable {
	variables := make([]*ssa.Variable, 0)
	if !containPoint {
		if variable := v.GetVariable(word); variable != nil {
			variables = append(variables, variable)
		} else {
			// log.Errorf("BUG: Value[%s] don't has variable[%s]", v, word)
		}
	} else {
		// fuzz match variable name
		_, lastName, _ := strings.Cut(word, ".")
		lastNameWithPoint := "." + lastName
		for _, variable := range v.GetAllVariables() {
			if !strings.HasSuffix(variable.GetName(), lastNameWithPoint) {
				continue
			}
			variables = append(variables, variable)
			break
		}
	}
	return variables
}

const (
	MAX_PHI_LEVEL = 5
)

func languageServerFind(prog *ssaapi.Program, word string, containPoint bool, v *ssaapi.Value, isReference bool) ([]*ssa.Variable, *ssa.Parameter) {
	var parameter *ssa.Parameter
	variables := make([]*ssa.Variable, 0)

	builtinFindVariable := func(v *ssaapi.Value) {
		variables = append(variables, findVariable(v, word, containPoint)...)
	}

	if v.IsFreeValue() {
		parameter = ssaapi.GetFreeValue(v)
		defaultValue := v.NewValue(parameter.GetDefault())
		// free value should find from raw value
		if defaultValue != nil {
			return languageServerFind(prog, word, containPoint, defaultValue, isReference)
		}
	}

	if isReference {
		// use
		var handler func(*ssaapi.Value, int)
		handler = func(value *ssaapi.Value, level int) {
			builtinFindVariable(value)
			if level == MAX_PHI_LEVEL {
				return
			}
			level++
			for _, user := range value.GetUsers() {
				handler(user, level)
			}
			for _, ref := range value.GetPointer() {
				handler(ref, level)
			}
			for _, mask := range value.GetMask() {
				handler(mask, level)
			}
		}
		handler(v, 0)
	}
	// def
	var handler func(*ssaapi.Value, int)
	handler = func(value *ssaapi.Value, level int) {
		builtinFindVariable(value)
		if level == MAX_PHI_LEVEL {
			return
		}
		level++
		// try to convert value to phi, add each edge variable
		if value.IsPhi() {
			for _, edge := range ssaapi.GetValues(value) {
				handler(edge, level)
			}
		}
	}
	handler(v, 0)

	variables = lo.Uniq(variables)

	return variables, parameter
}

func onFind(prog *ssaapi.Program, word string, containPoint bool, ssaRange *memedit.Range, v *ssaapi.Value, isReference bool) []*memedit.Range {
	ranges := make([]*memedit.Range, 0)
	if isReference {
		ranges = append(ranges, ssaRange)
	}
	variables, freeValue := languageServerFind(prog, word, containPoint, v, isReference)
	editor := ssaRange.GetEditor()

	if freeValue != nil && freeValue.IsFreeValue {
		// free value def is default value variable
		defValue := freeValue.GetDefault()
		if defValue != nil {
			var variables []*ssa.Variable
			if v, err := prog.NewValue(defValue); err == nil {
				variables = findVariable(v, word, containPoint)
			}
			if len(variables) > 0 && variables[0].DefRange != nil {
				ranges = append(ranges, editor.ExpandWordTextRange(variables[0].DefRange))
			}
		}

		// free value references
		if isReference {
			for _, variable := range variables {
				if variable.DefRange != nil {
					ranges = append(ranges, editor.ExpandWordTextRange(variable.DefRange))
				}

				variable.ForEachUseRange(func(rng *memedit.Range) {
					ranges = append(ranges, editor.ExpandWordTextRange(rng))
				})
			}
		}
	} else {
		for _, variable := range variables {
			if variable.DefRange != nil {
				ranges = append(ranges, editor.ExpandWordTextRange(variable.DefRange))
			}

			if isReference {
				variable.ForEachUseRange(func(rng *memedit.Range) {
					ranges = append(ranges, editor.ExpandWordTextRange(rng))
				})
			}
		}
	}

	// if extern variable, add extern variable range
	if v.IsExtern() && len(ranges) == 0 {
		ranges = append(ranges, editor.ExpandWordTextRange(v.GetRange()))
	}

	// sort by end offset
	sort.SliceStable(ranges, func(i, j int) bool {
		offset1 := editor.GetOffsetByPosition(ranges[i].GetEnd())
		offset2 := editor.GetOffsetByPosition(ranges[j].GetEnd())
		return offset1 < offset2
	})

	return ranges
}

func OnFindDefinition(prog *ssaapi.Program, word string, containPoint bool, ssaRange *memedit.Range, v *ssaapi.Value) []*memedit.Range {
	return onFind(prog, word, containPoint, ssaRange, v, false)
}

func OnFindReferences(prog *ssaapi.Program, word string, containPoint bool, ssaRange *memedit.Range, v *ssaapi.Value) []*memedit.Range {
	return onFind(prog, word, containPoint, ssaRange, v, true)
}

func RangeIfToGrpcRange(rng *memedit.Range) *ypb.Range {
	start, end := rng.GetStart(), rng.GetEnd()
	return &ypb.Range{
		StartLine:   int64(start.GetLine()),
		StartColumn: int64(start.GetColumn()),
		EndLine:     int64(end.GetLine()),
		EndColumn:   int64(end.GetColumn()),
	}
}

func (s *Server) YaklangLanguageFind(ctx context.Context, req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageFindResponse, error) {
	var (
		ranges []*memedit.Range
		err    error

		ret = &ypb.YaklangLanguageFindResponse{}
	)

	result, err := LanguageServerAnalyzeProgram(req)

	prog, word, containPoint, ssaRange, v := result.Program, result.Word, result.ContainPoint, result.Range, result.Value

	if err != nil {
		ret.Ranges = append(ret.Ranges, req.GetRange())
		log.Errorf("YaklangLanguageFind failed: %v", err)
		return ret, nil
	}

	switch req.InspectType {
	case DEFINITION:
		ranges = OnFindDefinition(prog, word, containPoint, ssaRange, v)
	case REFERENCES:
		ranges = OnFindReferences(prog, word, containPoint, ssaRange, v)
	}

	tmp := make(map[string]struct{})
	for _, rng := range ranges {
		hash := utils.CalcSha256(rng.GetStart().GetLine(), rng.GetStart().GetColumn(), rng.GetEnd().GetLine(), rng.GetEnd().GetColumn())
		if _, ok := tmp[hash]; ok {
			continue
		}
		tmp[hash] = struct{}{}
		ret.Ranges = append(ret.Ranges, RangeIfToGrpcRange(rng))
	}
	return ret, nil
}
