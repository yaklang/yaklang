package luaast

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// type errorType string
// type LuaMergeError []*LuaError
// type LuaError struct {
// 	StartPos Position
// 	EndPos   Position
// 	Message  string
// }

// func NewYakMergeError(mergeErrors ...LuaMergeError) *LuaMergeError {
// 	luaMergeError := &LuaMergeError{}
// 	luaMergeError.Merge(mergeErrors...)
// 	return luaMergeError
// }

// func (y *LuaMergeError) Push(e *LuaError) {
// 	*y = append(*y, e)
// }
// func (y *LuaMergeError) Merge(mergeErrors ...LuaMergeError) {
// 	for _, mergeError := range mergeErrors {
// 		*y = append(*y, mergeError...)
// 	}
// }
// func (y LuaMergeError) Error() string {
// 	errors := []string{}
// 	for _, yakError := range y {
// 		errors = append(errors, yakError.Error())
// 	}
// 	return strings.Join(errors, "\n")
// }
// func (e *LuaError) Error() string {
// 	return fmt.Sprintf("line %d:%d-%d:%d %s", e.StartPos.LineNumber, e.StartPos.ColumnNumber, e.EndPos.LineNumber, e.EndPos.ColumnNumber, e.Message)
// }
// func NewErrorWithPostion(msg string, start, end Position) *LuaError {
// 	return &LuaError{
// 		StartPos: start,
// 		EndPos:   end,
// 		Message:  msg,
// 	}
// }

// type ErrorListener struct {
// 	handler func(msg string, start, end Position)
// 	antlr.DefaultErrorListener
// }

// func (el *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
// 	if el.handler != nil {
// 		el.handler(msg, Position{LineNumber: line, ColumnNumber: column}, Position{LineNumber: line, ColumnNumber: column})
// 	}
// }

// func NewErrorListener(handler func(msg string, start, end Position)) *ErrorListener {
// 	return &ErrorListener{handler: handler}
// }

type ErrorStrategy struct {
	antlr.DefaultErrorStrategy
}

func NewErrorStrategy() *ErrorStrategy {
	return &ErrorStrategy{}
}

func (e *ErrorStrategy) ReportNoViableAlternative(recognizer antlr.Parser, n *antlr.NoViableAltException) {
}
