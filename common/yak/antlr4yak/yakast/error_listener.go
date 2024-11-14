package yakast

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

type errorType string

// type YakMergeError []*YakError
// type YakError struct {
// 	StartPos Position
// 	EndPos   Position
// 	Message  string
// }

// func NewYakMergeError(mergeErrors ...YakMergeError) *YakMergeError {
// 	yakMergeError := &YakMergeError{}
// 	yakMergeError.Merge(mergeErrors...)
// 	return yakMergeError
// }

// func (y *YakMergeError) Push(e *YakError) {
// 	*y = append(*y, e)
// }
// func (y *YakMergeError) Merge(mergeErrors ...YakMergeError) {
// 	for _, mergeError := range mergeErrors {
// 		*y = append(*y, mergeError...)
// 	}
// }
// func (y YakMergeError) Error() string {
// 	errors := []string{}
// 	for _, yakError := range y {
// 		errors = append(errors, yakError.Error())
// 	}
// 	return strings.Join(errors, "\n")
// }
// func (e *YakError) Error() string {
// 	return fmt.Sprintf("line %d:%d-%d:%d %s", e.StartPos.LineNumber, e.StartPos.ColumnNumber, e.EndPos.LineNumber, e.EndPos.ColumnNumber, e.Message)
// }
// func NewErrorWithPostion(msg string, start, end Position) *YakError {
// 	return &YakError{
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
