package static_analyzer

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

type SampleVerificationResult = sfanalysis.SampleVerificationResult

type SyntaxFlowCheckResult struct {
	SyntaxErrors    []*result.StaticAnalyzeResult
	FormattedErrors string
	Sample          *SampleVerificationResult
}

func SyntaxFlowRuleCheckingWithSample(code, sampleCode, filename, language string) SyntaxFlowCheckResult {
	opts := sfanalysis.DefaultOptions(sfanalysis.ProfileAISyntax)
	sampleCode = strings.TrimSpace(sampleCode)
	if sampleCode != "" && language != "" {
		opts.VerifySampleCode = true
		opts.SampleCode = sampleCode
		opts.SampleFilename = filename
		opts.SampleLanguage = language
	}

	report := sfanalysis.Analyze(context.Background(), code, opts)
	return SyntaxFlowCheckResult{
		SyntaxErrors:    report.SyntaxErrors,
		FormattedErrors: report.FormattedSyntaxErrors,
		Sample:          report.Sample,
	}
}

func FormatSyntaxFlowErrors(content string, errs []*result.StaticAnalyzeResult) string {
	return sfanalysis.FormatSyntaxFlowErrors(content, errs)
}
