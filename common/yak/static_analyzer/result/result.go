package result

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type MarkerSeverity string
type MarkerTag string

const (
	Error MarkerSeverity = "Error"
	Warn  MarkerSeverity = "Warning"
	Info  MarkerSeverity = "Info"
	Hint  MarkerSeverity = "Hint"
)

const (
	None        MarkerTag = ""
	Deprecated  MarkerTag = "Deprecated"
	Unnecessary MarkerTag = "Unnecessary"
)

type StaticAnalyzeResult struct {
	Message         string         `json:"message"`
	Severity        MarkerSeverity `json:"severity"`
	StartLineNumber int64          `json:"startLineNumber"`
	StartColumn     int64          `json:"startColumn"`
	EndLineNumber   int64          `json:"endLineNumber"`
	EndColumn       int64          `json:"endColumn"`
	Tag             MarkerTag      `json:"tag"`
	From            string         `json: "from"`
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
	res  []*StaticAnalyzeResult
	from string
}

func NewStaticAnalyzeResults(strs ...string) *StaticAnalyzeResults {
	from := ""
	if len(strs) > 0 {
		from = strs[0]
	}

	return &StaticAnalyzeResults{
		res:  make([]*StaticAnalyzeResult, 0),
		from: from,
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
	res := New(None, Error, message, v)
	res.From = e.from
	e.res = append(e.res, res)
}

// NewWarn in v.Range or [0:0-0:1]
func (e *StaticAnalyzeResults) NewWarn(message string, v *ssaapi.Value) {
	res := New(None, Warn, message, v)
	res.From = e.from
	e.res = append(e.res, res)
}

// NewDeprecated in v.Range or [0:0-0:1]
func (e *StaticAnalyzeResults) NewDeprecated(message string, v *ssaapi.Value) {
	res := New(Deprecated, Hint, message, v)
	res.From = e.from
	e.res = append(e.res, res)
}

// New Result
// Create Result from ssaapi.Value.Range,
// if v is nil, then create a result in file [0:0-0:1]
func New(tag MarkerTag, severity MarkerSeverity, message string, v *ssaapi.Value) *StaticAnalyzeResult {
	var ret *StaticAnalyzeResult
	if v == nil {
		ret = &StaticAnalyzeResult{
			StartLineNumber: 0,
			StartColumn:     0,
			EndLineNumber:   0,
			EndColumn:       1,
		}
	} else {
		r := v.GetRange()
		ret = &StaticAnalyzeResult{
			StartLineNumber: r.Start.Line,
			StartColumn:     r.Start.Column + 1,
			EndLineNumber:   r.End.Line,
			EndColumn:       r.End.Column + 1,
		}
	}
	ret.Message = message
	ret.Severity = severity
	if tag != None {
		ret.Tag = tag
	}

	return ret
}
