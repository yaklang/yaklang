package antlr4util

import (
	"bytes"
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

// error listener for lexer and parser
type ErrorListener struct {
	textFilter map[string]struct{}
	err        []string
	*antlr.DefaultErrorListener
}

func (el *ErrorListener) GetErrorString() string {
	return strings.Join(el.err, "\n")
}

func (el *ErrorListener) GetErrors() []string {
	return el.err
}

func (el *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	token, ok := offendingSymbol.(*antlr.CommonToken)
	var ctxText string
	var ctxTextHash string
	var start, end int
	if ok {
		stream := token.GetInputStream()
		start, end = token.GetStart(), token.GetStop()
		// get all code
		meditor := memedit.NewMemEditor(stream.GetText(0, stream.Size()))
		ctxText, _ = meditor.GetContextAroundRange(
			meditor.GetPositionByOffset(start),
			meditor.GetPositionByOffset(end),
			3,
			func(i int) string {
				return fmt.Sprintf("%5s| ", fmt.Sprint(i))
			},
		)
		if ctxText != "" {
			ctxTextHash = codec.Sha256(ctxText)
		}
		if el.textFilter == nil {
			el.textFilter = make(map[string]struct{})
		}
	}

	buf := bytes.NewBufferString("")
	if ctxText != "" {
		if el.textFilter != nil {
			_, existed := el.textFilter[ctxTextHash]
			if !existed {
				buf.WriteString("----" + utils.ShrinkString(ctxTextHash, 16) + "----\n")
				buf.WriteString(ctxText)
				buf.WriteByte('\n')
				buf.WriteString("-----------------------------\n")
				el.textFilter[ctxTextHash] = struct{}{}
			}
		}
	}
	buf.WriteString(fmt.Sprintf("line:%v:%v symbol: %v, reason: %v", line, column, offendingSymbol, msg))

	el.err = append(el.err, buf.String())
}

func NewErrorListener() *ErrorListener {
	return &ErrorListener{
		err:                  []string{},
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}
