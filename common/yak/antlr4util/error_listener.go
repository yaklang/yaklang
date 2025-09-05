package antlr4util

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type (
	SourceCodeErrors []*SourceCodeError
	SourceCodeError  struct {
		StartPos *memedit.Position
		EndPos   *memedit.Position
		Message  string
	}
)

func NewSourceCodeErrors(mergeErrors ...SourceCodeErrors) *SourceCodeErrors {
	err := &SourceCodeErrors{}
	err.Merge(mergeErrors...)
	return err
}

func (y *SourceCodeErrors) Push(e *SourceCodeError) {
	*y = append(*y, e)
}

func (y *SourceCodeErrors) Merge(mergeErrors ...SourceCodeErrors) {
	for _, mergeError := range mergeErrors {
		*y = append(*y, mergeError...)
	}
}

func (y SourceCodeErrors) Error() string {
	errors := []string{}
	for _, yakError := range y {
		errors = append(errors, yakError.Error())
	}
	return strings.Join(errors, "\n")
}

func (e *SourceCodeError) Error() string {
	return fmt.Sprintf("line %d:%d-%d:%d %s", e.StartPos.GetLine(), e.StartPos.GetColumn(), e.EndPos.GetLine(), e.EndPos.GetColumn(), e.Message)
}

func NewSourceCodeError(msg string, start, end *memedit.Position) *SourceCodeError {
	return &SourceCodeError{
		StartPos: start,
		EndPos:   end,
		Message:  msg,
	}
}

// error listener for lexer and parser
type ErrorListener struct {
	*antlr.DefaultErrorListener
	textFilter map[string]struct{}
	handler    handlerFunc
	err        []string
}

type handlerFunc func(self *ErrorListener, recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException)

func (el *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	if el.handler != nil {
		el.handler(el, recognizer, offendingSymbol, line, column, msg, e)
	}
}

func (el *ErrorListener) GetErrorString() string {
	return strings.Join(el.err, "\n")
}
func (el *ErrorListener) Error() error {
	if len(el.err) == 0 {
		return nil
	}
	return fmt.Errorf("syntax errors found:\n%s", el.GetErrorString())
}

func (el *ErrorListener) GetErrors() []string {
	return el.err
}

func NewErrorListener(handlers ...handlerFunc) *ErrorListener {
	var handler handlerFunc
	if len(handlers) == 0 {
		// default handler
		handler = StringSyntaxErrorHandler
	} else {
		handler = handlers[0]
	}
	return &ErrorListener{
		handler:              handler,
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}

func SimpleSyntaxErrorHandler(simpleHandler func(msg string, start, end *memedit.Position)) handlerFunc {
	return func(el *ErrorListener, recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
		if el.handler != nil {
			// token, ok := offendingSymbol.(*antlr.CommonToken)
			// if ok {
			// 	simpleHandler(msg, token, token)
			// } else {
			position := memedit.NewPosition(line, column)
			simpleHandler(msg, position, position)
			// }
		}
	}
}

func StringSyntaxErrorHandler(el *ErrorListener, recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
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
