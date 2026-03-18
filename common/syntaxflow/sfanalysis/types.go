package sfanalysis

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Profile string

const (
	ProfileEditor   Profile = "editor"
	ProfileAISyntax Profile = "ai_syntax"
	ProfileQuality  Profile = "quality"
	ProfileVerify   Profile = "verify"
)

type VerifyOption func(*verifyConfig)

type Options struct {
	Profile             Profile
	AllowBlank          bool
	NeedFormattedSyntax bool
	CheckMetadata       bool
	CheckRuleLogic      bool
	NeedScore           bool
	VerifyEmbeddedTests bool
	VerifyOptions       []VerifyOption
	VerifySampleCode    bool
	SampleCode          string
	SampleFilename      string
	SampleLanguage      string
}

func DefaultVerifyOptions(profile Profile) []VerifyOption {
	switch profile {
	case ProfileQuality:
		return []VerifyOption{WithVerifyNegative(true)}
	case ProfileVerify:
		return []VerifyOption{WithVerifyNegative(true)}
	default:
		return nil
	}
}

func DefaultOptions(profile Profile) Options {
	switch profile {
	case ProfileAISyntax:
		return Options{
			Profile:             profile,
			AllowBlank:          true,
			NeedFormattedSyntax: true,
		}
	case ProfileQuality:
		return Options{
			Profile:             profile,
			CheckMetadata:       true,
			CheckRuleLogic:      true,
			NeedScore:           true,
			VerifyEmbeddedTests: true,
			VerifyOptions:       DefaultVerifyOptions(profile),
		}
	case ProfileVerify:
		return Options{
			Profile:             profile,
			VerifyEmbeddedTests: true,
			VerifyOptions:       DefaultVerifyOptions(profile),
		}
	default:
		return Options{
			Profile:             ProfileEditor,
			AllowBlank:          true,
			NeedFormattedSyntax: true,
		}
	}
}

type Report struct {
	Profile               Profile
	Code                  string
	IsBlank               bool
	Frame                 *sfvm.SFFrame
	SyntaxErrors          []*result.StaticAnalyzeResult
	FormattedSyntaxErrors string
	Quality               *SyntaxFlowRuleAnalyzeResult
	EmbeddedVerify        *EmbeddedVerifyReport
	Sample                *SampleVerificationResult
}

type EmbeddedVerifyReport struct {
	Passed            bool
	Error             error
	PositiveTestCount int
	NegativeTestCount int
}

type SyntaxFlowRuleAnalyzeResult struct {
	Score    int                     `json:"score"`
	MaxScore int                     `json:"max_score"`
	Problems []SyntaxFlowRuleProblem `json:"problems"`
}

type SyntaxFlowRuleProblem struct {
	Type        string     `json:"type"`
	Severity    string     `json:"severity"`
	Description string     `json:"description"`
	Suggestion  string     `json:"suggestion"`
	Range       *ypb.Range `json:"range"`
}

type SampleVerificationResult struct {
	Matched              bool           `json:"matched"`
	Message              string         `json:"message"`
	Error                string         `json:"error,omitempty"`
	AlertCount           int            `json:"alert_count,omitempty"`
	AlertDetails         map[string]int `json:"alert_details,omitempty"`
	QueryResultsFull     string         `json:"query_results_full,omitempty"`
	Suggestion           string         `json:"suggestion,omitempty"`
	ResultVarsDiagnostic map[string]int `json:"result_vars_diagnostic,omitempty"`
	DiagnosticHint       string         `json:"diagnostic_hint,omitempty"`
}

func (s *SyntaxFlowRuleAnalyzeResult) GetResponse() *ypb.SmokingEvaluatePluginResponse {
	res := &ypb.SmokingEvaluatePluginResponse{
		Score: int64(s.Score),
	}
	res.Results = make([]*ypb.SmokingEvaluateResult, 0, len(s.Problems))
	for _, problem := range s.Problems {
		res.Results = append(res.Results, &ypb.SmokingEvaluateResult{
			Item:       problem.Type,
			Suggestion: problem.Suggestion,
			Range:      problem.Range,
			Severity:   problem.Severity,
		})
	}
	return res
}

func (s *SyntaxFlowRuleAnalyzeResult) GetStaticAnalyzeResults() []*result.StaticAnalyzeResult {
	results := make([]*result.StaticAnalyzeResult, 0, len(s.Problems))
	for _, problem := range s.Problems {
		severity := result.Warn
		switch problem.Severity {
		case Error:
			severity = result.Error
		case Info:
			severity = result.Info
		}
		res := &result.StaticAnalyzeResult{
			Message:     problem.Description,
			Severity:    severity,
			From:        "syntaxflow_quality",
			ScoreOffset: -scorePenalty(problem.Type),
		}
		if problem.Range != nil {
			res.StartLineNumber = problem.Range.StartLine
			res.StartColumn = problem.Range.StartColumn
			res.EndLineNumber = problem.Range.EndLine
			res.EndColumn = problem.Range.EndColumn
		} else {
			res.StartLineNumber = 0
			res.StartColumn = 0
			res.EndLineNumber = 0
			res.EndColumn = 1
		}
		results = append(results, res)
	}
	return results
}

func scorePenalty(problemType string) int {
	switch problemType {
	case ProblemTypeSyntaxError:
		return SyntaxErrorPenalty
	case ProblemTypeBlankRule:
		return BlankRulePenalty
	case ProblemTypeLackDescriptionField:
		return MissingDescriptionPenalty
	case ProblemTypeLackSolutionField:
		return MissingSolutionPenalty
	case ProblemTypeMissingPositiveTestData:
		return MissingPositiveTestPenalty
	case ProblemTypeMissingNegativeTestData:
		return MissingNegativeTestPenalty
	case ProblemTypeTestCaseNotPass:
		return VerifyTestCaseNotPassPenalty
	case ProblemTypeMissingAlert:
		return MissingAlertPenalty
	default:
		return 0
	}
}
