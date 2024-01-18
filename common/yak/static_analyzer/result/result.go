package result

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type StaticAnalyzeResult struct {
	Message         string `json:"message"`
	Severity        string `json:"severity"` // Error / Warning
	StartLineNumber int64  `json:"startLineNumber"`
	StartColumn     int64  `json:"startColumn"`
	EndLineNumber   int64  `json:"endLineNumber"`
	EndColumn       int64  `json:"endColumn"`
	Tag             string `json:"tag"`
	From            string `json: "from"`
}

func (e *StaticAnalyzeResult) String() string {
	return fmt.Sprintf("[%s]: %s in [%d:%d -- %d:%d] from %s\n",
		e.Severity, e.Message,
		e.StartLineNumber, e.StartColumn,
		e.EndLineNumber, e.EndColumn,
		e.From,
	)
}

type StaticAnalyzeResults struct {
	res []*StaticAnalyzeResult
}

func NewStaticAnalyzeResults() *StaticAnalyzeResults {
	return &StaticAnalyzeResults{
		res: make([]*StaticAnalyzeResult, 0),
	}
}

// Get all Result
func (e *StaticAnalyzeResults) Get() []*StaticAnalyzeResult {
	return e.res
}

// Merge another result
func (e *StaticAnalyzeResults) Merge(o *StaticAnalyzeResults) {
	e.res = append(e.res, o.res...)
}

// NewError in v.Range or [0:0-0:1]
func (e *StaticAnalyzeResults) NewError(message string, v *ssaapi.Value) {
	e.res = append(e.res, New("", "Error", message, v))
}

// NewWarn in v.Range or [0:0-0:1]
func (e *StaticAnalyzeResults) NewWarn(message string, v *ssaapi.Value) {
	e.res = append(e.res, New("", "Warning", message, v))
}

// NewDeprecated in v.Range or [0:0-0:1]
func (e *StaticAnalyzeResults) NewDeprecated(message string, v *ssaapi.Value) {
	e.res = append(e.res, New("Deprecated", "", message, v))
}

// New Result
// Create Result from ssaapi.Value.Range,
// if v is nil, then create a result in file [0:0-0:1]
func New(tag, severity, message string, v *ssaapi.Value) *StaticAnalyzeResult {
	if v == nil {
		return &StaticAnalyzeResult{
			Message:         message,
			Severity:        severity,
			StartLineNumber: 0,
			StartColumn:     0,
			EndLineNumber:   0,
			EndColumn:       1,
			Tag:             tag,
			From:            "",
		}
	}

	r := v.GetRange()
	return &StaticAnalyzeResult{
		Message:         message,
		Severity:        severity,
		StartLineNumber: r.Start.Line,
		StartColumn:     r.Start.Column,
		EndLineNumber:   r.End.Line,
		EndColumn:       r.End.Column,
		Tag:             tag,
		From:            "",
	}
}
