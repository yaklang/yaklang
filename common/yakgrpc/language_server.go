package yakgrpc

import (
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
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
	Word         string
	ContainPoint bool
	Range        *ssa.Range
	Value        *ssaapi.Value
	Editor       *memedit.MemEditor
}

func LanguageServerAnalyzeProgram(code string, inspectType, scriptType string, rng *ypb.Range) (*LanguageServerAnalyzerResult, error) {
	ssaRange := GrpcRangeToSSARange(code, rng)
	editor := ssaRange.GetEditor()
	rangeWordText := ssaRange.GetWordText()
	word, containPoint := trimSourceCode(rangeWordText)

	getProgram := func() (*ssaapi.Program, error) {
		prog, err := static_analyzer.SSAParse(code, scriptType)
		if err == nil {
			return prog, nil
		}

		startOffset, endOffset := ssaRange.GetOffset(), ssaRange.GetEndOffset()
		shouldTrim := containPoint
		fixRange := true
		if !containPoint && editor.GetTextFromOffset(endOffset, endOffset+1) == "." {
			// fix for hover or signature
			fixRange = false
			shouldTrim = true
			rangeWordText = editor.GetWordTextFromOffset(startOffset, endOffset+1)
		}

		// try to remove content after point
		if shouldTrim {
			offset := startOffset - 1
			if offset < 0 {
				offset = 0
			}
			lastIndex := strings.LastIndex(rangeWordText, ".")
			before, after := rangeWordText[:lastIndex], rangeWordText[lastIndex+1:]
			trimCode := code[:offset] + strings.Replace(code[offset:], rangeWordText, before, 1)

			prog, err = static_analyzer.SSAParse(trimCode, scriptType)
			if err == nil {
				// reset ssaRange and editor
				newEditor, ok := prog.Program.GetEditor("")
				if !ok {
					newEditor = memedit.NewMemEditor(trimCode)
				}
				// end use old editor to get position
				if fixRange {
					ssaRange = ssa.NewRange(newEditor, ssaRange.GetStart(), editor.GetPositionByOffset(endOffset-len(after)-1))
				}
				editor = newEditor

				return prog, nil
			}
		}

		// try ignore syntax error
		prog, err = static_analyzer.SSAParse(code, scriptType, ssaapi.WithIgnoreSyntaxError())

		return prog, err
	}

	prog, err := getProgram()
	if err != nil {
		log.Error(err)
		return nil, errors.New("ssa parse error")
	}

	// prog.Program.ShowOffsetMap()

	v := getFrontValueByOffset(prog, editor, ssaRange, 0)
	// fallback
	if v == nil {
		v = getSSAValueByPosition(prog, word, ssaRange)
	}

	return &LanguageServerAnalyzerResult{
		Program:      prog,
		Word:         word,
		ContainPoint: containPoint,
		Range:        ssaRange,
		Value:        v,
		Editor:       editor,
	}, nil
}
