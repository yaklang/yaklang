package yakgrpc

import (
	"context"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func languageServerFind(prog *ssaapi.Program, word string, containPoint bool, v *ssaapi.Value, isReference bool) []*ssa.Variable {
	variables := make([]*ssa.Variable, 0)

	findVariable := func(v *ssaapi.Value) {
		if !containPoint {
			variables = append(variables, v.GetVariable(word))
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
	}

	if isReference {
		findVariable(v)
		// try to get users phi, add each edge variable
		for _, user := range v.GetUsers() {
			if !user.IsPhi() {
				continue
			}

			findVariable(user)
		}
		// try to convert value to phi, add each edge variable
		if v.IsPhi() {
			for _, edge := range ssaapi.GetValues(v) {
				findVariable(edge)
			}
		}
	} else {
		findVariable(v)
		// try to convert value to phi, add each edge variable
		if v.IsPhi() {
			for _, edge := range ssaapi.GetValues(v) {
				findVariable(edge)
			}
		}
	}

	variables = lo.Uniq(variables)

	return variables
}

func onFind(prog *ssaapi.Program, word string, containPoint bool, ssaRange *ssa.Range, v *ssaapi.Value, isReference bool) ([]memedit.RangeIf, error) {
	ranges := make([]memedit.RangeIf, 0)
	variables := languageServerFind(prog, word, containPoint, v, isReference)
	editor := ssaRange.GetEditor()

	for _, variable := range variables {
		if variable.DefRange != nil {
			ranges = append(ranges, editor.ExpandWordTextRange(variable.DefRange))
		}

		if isReference {
			for rng := range variable.UseRange {
				ranges = append(ranges, editor.ExpandWordTextRange(rng))
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

	return ranges, nil
}

func OnFindDefinition(prog *ssaapi.Program, word string, containPoint bool, ssaRange *ssa.Range, v *ssaapi.Value) ([]memedit.RangeIf, error) {
	return onFind(prog, word, containPoint, ssaRange, v, false)
}

func OnFindReferences(prog *ssaapi.Program, word string, containPoint bool, ssaRange *ssa.Range, v *ssaapi.Value) ([]memedit.RangeIf, error) {
	return onFind(prog, word, containPoint, ssaRange, v, true)
}

func RangeIfToGrpcRange(rng memedit.RangeIf) *ypb.Range {
	start, end := rng.GetStart(), rng.GetEnd()
	return &ypb.Range{
		StartLine:   int64(start.GetLine()),
		StartColumn: int64(start.GetColumn() + 1),
		EndLine:     int64(end.GetLine()),
		EndColumn:   int64(end.GetColumn() + 1),
	}
}

func (s *Server) YaklangLanguageFind(ctx context.Context, req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageFindResponse, error) {
	var (
		ranges []memedit.RangeIf
		err    error

		ret = &ypb.YaklangLanguageFindResponse{}
	)

	result, err := LanguageServerAnalyzeProgram(req.GetYakScriptCode(), req.GetInspectType(), req.GetYakScriptType(), req.GetRange())

	prog, word, containPoint, ssaRange, v := result.Program, result.Word, result.ContainPoint, result.Range, result.Value

	if err != nil {
		return ret, err
	}

	switch req.InspectType {
	case DEFINITION:
		ranges, err = OnFindDefinition(prog, word, containPoint, ssaRange, v)
	case REFERENCES:
		ranges, err = OnFindReferences(prog, word, containPoint, ssaRange, v)
	}

	for _, rng := range ranges {
		ret.Ranges = append(ret.Ranges, RangeIfToGrpcRange(rng))
	}

	return ret, nil
}
