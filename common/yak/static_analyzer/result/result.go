package result

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type (
	MarkerSeverity string
	MarkerTag      string
)

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
	From            string         `json:"from"`
	ScoreOffset     int            `json:"scoreOffset"`
}

func (e *StaticAnalyzeResult) SetNegativeScore(score int) {
	e.ScoreOffset = 0 - score
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
func (e *StaticAnalyzeResults) NewError(message string, v *ssaapi.Value) *StaticAnalyzeResult {
	res := New(None, Error, message, v)
	if res == nil {
		return nil
	}
	res.From = e.from
	e.res = append(e.res, res)
	return res
}

// NewWarn in v.Range or [0:0-0:1]
func (e *StaticAnalyzeResults) NewWarn(message string, v *ssaapi.Value) *StaticAnalyzeResult {
	res := New(None, Warn, message, v)
	if res == nil {
		return nil
	}
	res.From = e.from
	e.res = append(e.res, res)
	return res
}

// NewDeprecated in v.Range or [0:0-0:1]
func (e *StaticAnalyzeResults) NewDeprecated(message string, v *ssaapi.Value) *StaticAnalyzeResult {
	res := New(Deprecated, Hint, message, v)
	if res == nil {
		return nil
	}
	res.From = e.from
	e.res = append(e.res, res)
	return res
}

// New Result
// Create Result from ssaapi.Value.Range,
// if v is nil, then create a result in file [0:0-0:1]
func New(tag MarkerTag, severity MarkerSeverity, message string, v *ssaapi.Value) *StaticAnalyzeResult {
	if ssa.ShouldSkipError(v.GetSSAInst()) {
		return nil
	}
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
			StartLineNumber: int64(r.GetStart().GetLine()),
			StartColumn:     int64(r.GetStart().GetColumn()) + 1,
			EndLineNumber:   int64(r.GetEnd().GetLine()),
			EndColumn:       int64(r.GetEnd().GetColumn()) + 1,
		}
	}
	ret.Message = message
	ret.Severity = severity
	if tag != None {
		ret.Tag = tag
	}

	return ret
}

func CalculateScoreFromResults(results []*StaticAnalyzeResult) int {
	score := 100
	for _, sRes := range results {
		switch sRes.Severity {
		case Error:
			if sRes.ScoreOffset == 0 {
				score -= 100 // if no score offset, score will be 0
			} else {
				score += sRes.ScoreOffset // use score offset
			}
		case Warn:
			if sRes.ScoreOffset == 0 {
				if score > 60 {
					score = 60 // if no score offset, score will be 60
				}
			} else {
				score += sRes.ScoreOffset // use score offset
			}
		}
	}

	if score < 0 {
		score = 0
	}

	return score
}
