package yakgrpc

import (
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	COMPLETION = "completion"
	HOVER      = "hover"
	SIGNATURE  = "signature"
	DEFINITION = "definition"
	REFERENCES = "reference"
)

type LanguageServerAnalyzerResult struct {
	Program      *ssaapi.Program
	Range        *memedit.Range
	Value        *ssaapi.Value
	Editor       *memedit.MemEditor
	Word         string
	ContainPoint bool
	PointSuffix  bool
}

func (r *LanguageServerAnalyzerResult) Clone() *LanguageServerAnalyzerResult {
	return &LanguageServerAnalyzerResult{
		Program:      r.Program,
		Range:        r.Range,
		Value:        r.Value,
		Editor:       r.Editor,
		Word:         r.Word,
		ContainPoint: r.ContainPoint,
		PointSuffix:  r.PointSuffix,
	}
}

var fallbackAnalyzeCache = utils.NewTTLCache[*LanguageServerAnalyzerResult](30 * time.Second)

func LanguageServerAnalyzeProgram(req *ypb.YaklangLanguageSuggestionRequest) (*LanguageServerAnalyzerResult, error) {
	// from database
	if programName := req.GetProgramName(); programName != "" {
		return languageServerAnalyzeFromDatabase(req)
	}
	return languageServerAnalyzeFromSource(req)
}

func languageServerAnalyzeFromDatabase(req *ypb.YaklangLanguageSuggestionRequest) (*LanguageServerAnalyzerResult, error) {
	ret := &LanguageServerAnalyzerResult{}
	// get  program
	programName := req.GetProgramName()
	if prog, err := ssaapi.FromDatabase(programName); err != nil {
		return ret, err
	} else {
		ret.Program = prog
	}

	// get editor
	fileName := req.GetFileName()
	editor, err := ssadb.GetEditorByFileName(fileName)
	if err != nil {
		return ret, err
	}
	// get range
	rng := req.GetRange()
	SSARange := editor.GetRangeByPosition(
		editor.GetPositionByLine(int(rng.StartLine), int(rng.StartColumn)),
		editor.GetPositionByLine(int(rng.EndLine), int(rng.EndColumn)),
	)
	ret.Range = SSARange

	// word
	ret.Word = SSARange.GetText()

	// value
	valueID, err := ssadb.GetValueBeforeEndOffset(ssadb.GetDB(), SSARange)
	if err != nil {
		return ret, err
	}
	if value, err := ssa.NewLazyInstruction(ret.Program.Program, valueID); err != nil && !utils.IsNil(value) {
		return ret, err
	} else {
		if v, err := ret.Program.NewValue(value); err == nil {
			ret.Value = v
		}
	}

	return ret, nil
}

func languageServerAnalyzeFromSource(req *ypb.YaklangLanguageSuggestionRequest) (*LanguageServerAnalyzerResult, error) {
	// from source code
	code := req.GetYakScriptCode()
	rng := req.GetRange()
	scriptType := req.GetYakScriptType()
	id := req.GetModelID()

	ssaRange := GrpcRangeToSSARange(code, rng)
	editor := ssaRange.GetEditor()
	rangeWordText := ssaRange.GetWordText()
	word, containPoint, pointSuffix := trimSourceCode(rangeWordText)

	getProgram := func() (*ssaapi.Program, error) {
		prog, err := static_analyzer.SSAParse(code, scriptType)
		if err == nil {
			return prog, nil
		}

		startOffset, endOffset := ssaRange.GetStartOffset(), ssaRange.GetEndOffset()
		shouldTrim := containPoint
		fixRange := true
		if !containPoint && editor.GetTextFromOffset(endOffset, endOffset+1) == "." {
			// fix for hover or signature
			fixRange = false
			shouldTrim = true
			endOffset++
			rangeWordText = editor.GetWordTextFromOffset(startOffset, endOffset)
		}

		// try to remove content after point
		if shouldTrim {
			trimCode := editor.GetTextFromOffset(0, endOffset-1)
			trimCode += editor.GetTextFromOffset(endOffset, editor.CodeLength())

			prog, err = static_analyzer.SSAParse(trimCode, scriptType)
			if err == nil {
				// reset ssaRange and editor
				newEditor, ok := prog.Program.GetEditor("")
				if !ok {
					newEditor = memedit.NewMemEditor(trimCode)
				}
				if fixRange {
					ssaRange = newEditor.GetRangeOffset(ssaRange.GetStartOffset(), endOffset-1)
				}
				editor = newEditor

				return prog, nil
			}
		}
		if err != nil {
			prog, err = static_analyzer.SSAParse(code, scriptType, ssaapi.WithIgnoreSyntaxError(true))
		}
		return prog, err
	}

	prog, err := getProgram()
	if err != nil {
		if fallback, ok := fallbackAnalyzeCache.Get(id); ok {
			cloned := fallback.Clone()
			cloned.ContainPoint = containPoint
			cloned.PointSuffix = pointSuffix
			cloned.Range = ssaRange
			cloned.Word = word
			cloned.Value = nil

			return cloned, nil
		} else {
			return nil, utils.Wrap(err, "language server analyze program error")
		}
	}

	// prog.Program.ShowOffsetMap()

	v := getFrontValueByOffset(prog, editor, ssaRange, 0)
	// fallback
	if v == nil {
		v = getSSAValueByPosition(prog, word, ssaRange)
	}

	result := &LanguageServerAnalyzerResult{
		Program:      prog,
		Word:         word,
		ContainPoint: containPoint,
		Range:        ssaRange,
		Value:        v,
		Editor:       editor,
		PointSuffix:  pointSuffix,
	}
	fallbackAnalyzeCache.Set(id, result)
	return result, err
}

func GrpcRangeToSSARange(sourceCode string, r *ypb.Range) *memedit.Range {
	e := memedit.NewMemEditor(sourceCode)
	return e.GetRangeByPosition(
		e.GetPositionByLine(int(r.StartLine), int(r.StartColumn)),
		e.GetPositionByLine(int(r.EndLine), int(r.EndColumn)),
	)
}

func getFrontValueByOffset(prog *ssaapi.Program, editor *memedit.MemEditor, rng *memedit.Range, skipNum int) *ssaapi.Value {
	// use editor instead of prog.Program.Editor because of ssa cache
	var value ssa.Value
	offset := rng.GetEndOffset()
	for i := 0; i < skipNum; i++ {
		_, offset = prog.Program.SearchIndexAndOffsetByOffset(offset)
		offset--
	}
	_, value = prog.Program.GetFrontValueByOffset(offset)
	if !utils.IsNil(value) {
		if v, err := prog.NewValue(value); err == nil {
			return v
		}
	}
	return nil
}

// Deprecated: now can get the closest value
func getSSAValueByPosition(prog *ssaapi.Program, sourceCode string, position *memedit.Range) *ssaapi.Value {
	var values ssaapi.Values
	for i, word := range strings.Split(sourceCode, ".") {
		if i == 0 {
			values = prog.Ref(word)
		} else {
			// fallback
			newValues := values.Ref(word)
			if len(newValues) == 0 {
				break
			} else {
				values = newValues
			}
		}
	}
	values = sortValuesByPosition(values, position)
	if len(values) == 0 {
		return nil
	}
	return values[0].GetSelf()
}

// Deprecated: now can get the closest value
func getSSAParentValueByPosition(prog *ssaapi.Program, sourceCode string, position *memedit.Range) *ssaapi.Value {
	word := strings.Split(sourceCode, ".")[0]
	values := prog.Ref(word).Filter(func(v *ssaapi.Value) bool {
		position2 := v.GetRange()
		if position2 == nil {
			return false
		}
		if position2.GetStart().GetLine() > position.GetStart().GetLine() {
			return false
		}
		return true
	})
	values = sortValuesByPosition(values, position)
	if len(values) == 0 {
		return nil
	}
	return values[0].GetSelf()
}

func sortValuesByPosition(values ssaapi.Values, position *memedit.Range) ssaapi.Values {
	// todo: 需要修改SSA，需要真正的RefLocation
	values = values.Filter(func(v *ssaapi.Value) bool {
		position2 := v.GetRange()
		if position2 == nil {
			return false
		}
		if position2.GetStart().GetLine() > position.GetStart().GetLine() {
			return false
		}
		return true
	})
	sort.SliceStable(values, func(i, j int) bool {
		line1, line2 := values[i].GetRange().GetStart().GetLine(), values[j].GetRange().GetStart().GetLine()
		if line1 == line2 {
			return values[i].GetRange().GetStart().GetColumn() > values[j].GetRange().GetStart().GetColumn()
		} else {
			return line1 > line2
		}
	})
	return values
}
