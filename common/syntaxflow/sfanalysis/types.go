package sfanalysis

import (
	"strings"

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

type Option func(*config)

type config struct {
	profile             Profile
	needFormattedSyntax bool
	checkMetadata       bool
	checkRuleLogic      bool
	needScore           bool
	verifyEmbeddedTests bool
	verifySampleCode    bool
	sampleCode          string
	sampleFilename      string
	sampleLanguage      string
	requirePositive     bool
	requireNegative     bool
	verifyNegative      bool
	strictAlertHigh     bool
}

func newConfig(opts ...Option) config {
	cfg := config{}
	WithProfile(ProfileEditor)(&cfg)
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}
	return cfg
}

func WithProfile(profile Profile) Option {
	return func(c *config) {
		c.profile = profile
		c.needFormattedSyntax = false
		c.checkMetadata = false
		c.checkRuleLogic = false
		c.needScore = false
		c.verifyEmbeddedTests = false
		c.requirePositive = false
		c.requireNegative = false
		c.verifyNegative = false
		c.strictAlertHigh = false

		switch profile {
		case ProfileAISyntax:
			c.needFormattedSyntax = true
		case ProfileQuality:
			c.checkMetadata = true
			c.checkRuleLogic = true
			c.needScore = true
			c.verifyEmbeddedTests = true
			c.verifyNegative = true
		case ProfileVerify:
			c.verifyEmbeddedTests = true
			c.verifyNegative = true
		case ProfileEditor, "":
			c.profile = ProfileEditor
			c.needFormattedSyntax = true
		default:
			c.needFormattedSyntax = true
		}
	}
}

func WithSampleVerification(sampleCode, sampleFilename, sampleLanguage string) Option {
	return func(c *config) {
		c.verifySampleCode = strings.TrimSpace(sampleCode) != "" && strings.TrimSpace(sampleLanguage) != ""
		c.sampleCode = sampleCode
		c.sampleFilename = sampleFilename
		c.sampleLanguage = sampleLanguage
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
