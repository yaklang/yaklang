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

type LanguageServerAnalyzerResult struct {
	Program      *ssaapi.Program
	Word         string
	ContainPoint bool
	Range        *ssa.Range
	Value        *ssaapi.Value
	Editor       *memedit.MemEditor
}

func (r *LanguageServerAnalyzerResult) Release() {
	r.Editor.Release()
}

func LanguageServerAnalyzeProgram(code string, scriptType string, rng *ypb.Range) (*LanguageServerAnalyzerResult, error) {
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
		if containPoint {
			endOffset := ssaRange.GetEndOffset()
			_, after, _ := strings.Cut(rangeWordText, ".")
			startOffset := endOffset - (len(after) + 1)

			trimCode := code[:startOffset] + code[endOffset:]
			prog, err = ssaapi.Parse(trimCode, opt...)
			if err == nil {
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
