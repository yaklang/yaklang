package yakgrpc

import (
	"context"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func languageServerFind(prog *ssaapi.Program, word string, containPoint bool, v *ssaapi.Value) *ssa.Variable {
	var targetVariable *ssa.Variable

	if !containPoint {
		targetVariable = v.GetVariable(word)
	} else {
		_, lastName, _ := strings.Cut(word, ".")
		lastNameWithPoint := "." + lastName
		variables := v.GetAllVariables()
		for _, variable := range variables {
			if !strings.HasSuffix(variable.GetName(), lastNameWithPoint) {
				continue
			}
			targetVariable = variable
			break
		}
	}

	// if v.IsExtern() {
	// }

	return targetVariable
}

func OnFindDefinition(prog *ssaapi.Program, word string, containPoint bool, ssaRange *ssa.Range, v *ssaapi.Value) ([]memedit.RangeIf, error) {
	ranges := make([]memedit.RangeIf, 0)
	targetVariable := languageServerFind(prog, word, containPoint, v)

	if targetVariable != nil {
		editor := ssaRange.GetEditor()
		ranges = append(ranges, editor.ExpandWordTextRange(targetVariable.DefRange))
	} else if v.IsExtern() {
		ranges = append(ranges, v.GetRange())
	}
	return ranges, nil
}

func OnFindReferences(prog *ssaapi.Program, word string, containPoint bool, ssaRange *ssa.Range, v *ssaapi.Value) ([]memedit.RangeIf, error) {
	ranges := make([]memedit.RangeIf, 0)
	targetVariable := languageServerFind(prog, word, containPoint, v)

	if targetVariable != nil {
		editor := ssaRange.GetEditor()
		ranges = append(ranges, editor.ExpandWordTextRange(targetVariable.DefRange))
		for rng := range targetVariable.UseRange {
			ranges = append(ranges, editor.ExpandWordTextRange(rng))
		}

		// sort by end offset
		sort.SliceStable(ranges, func(i, j int) bool {
			offset1 := editor.GetOffsetByPosition(ranges[i].GetEnd())
			offset2 := editor.GetOffsetByPosition(ranges[j].GetEnd())
			return offset1 < offset2
		})
	}

	return ranges, nil
}

func RangeIfToGrpcRange(rng memedit.RangeIf) *ypb.Range {
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
		ranges []memedit.RangeIf
		err    error

		ret = &ypb.YaklangLanguageFindResponse{}
	)

	result, err := LanguageServerAnalyzeProgram(req.GetYakScriptCode(), req.GetYakScriptType(), req.GetRange())
	defer result.Release()

	prog, word, containPoint, ssaRange, v := result.Program, result.Word, result.ContainPoint, result.Range, result.Value

	if err != nil {
		return ret, err
	}

	switch req.InspectType {
	case "definition":
		ranges, err = OnFindDefinition(prog, word, containPoint, ssaRange, v)
	case "reference":
		ranges, err = OnFindReferences(prog, word, containPoint, ssaRange, v)
	}

	for _, rng := range ranges {
		ret.Ranges = append(ret.Ranges, RangeIfToGrpcRange(rng))
	}

	return ret, nil
}
