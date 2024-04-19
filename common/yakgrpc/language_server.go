package yakgrpc

import (
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	pta "github.com/yaklang/yaklang/common/yak/static_analyzer"
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

	opt := pta.GetPluginSSAOpt(scriptType)

	getProgram := func() (*ssaapi.Program, error) {
		prog, err := ssaapi.Parse(code, opt...)
		if err == nil {
			return prog, nil
		}

		// try to remove content after point
		if containPoint && inspectType == COMPLETION {
			offset, endOffset := ssaRange.GetOffset()-1, ssaRange.GetEndOffset()
			before, after, _ := strings.Cut(rangeWordText, ".")
			trimCode := code[:offset] + strings.Replace(code[offset:], rangeWordText, before, 1)

			prog, err = ssaapi.Parse(trimCode, opt...)
			if err == nil {
				// reset ssaRange and editor
				newEditor := prog.Program.GetCurrentEditor()
				// end use old editor to get position
				ssaRange = ssa.NewRange(newEditor, ssaRange.GetStart(), editor.GetPositionByOffset(endOffset-len(after)-1))
				editor = newEditor

				return prog, nil
			}
		}

		// try ignore syntax error
		opt = append(opt, ssaapi.WithIgnoreSyntaxError(true))
		prog, err = ssaapi.Parse(code, opt...)

		return prog, err
	}

	prog, err := getProgram()
	if err != nil {
		log.Error(err)
		return nil, errors.New("ssa parse error")
	}

	// todo: remove this
	// prog.Program.ShowOffsetMap()

	v := getFrontValueByOffset(prog, editor, ssaRange)
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
