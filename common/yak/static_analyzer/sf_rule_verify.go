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
	opts := []sfanalysis.Option{sfanalysis.WithProfile(sfanalysis.ProfileAISyntax)}
	sampleCode = strings.TrimSpace(sampleCode)
	if sampleCode != "" && language != "" {
		opts = append(opts, sfanalysis.WithSampleVerification(sampleCode, filename, language))
	}

	report := sfanalysis.Analyze(context.Background(), code, opts...)
	return SyntaxFlowCheckResult{
		SyntaxErrors:    report.SyntaxErrors,
		FormattedErrors: report.FormattedSyntaxErrors,
		Sample:          report.Sample,
	}
}

func FormatSyntaxFlowErrors(content string, errs []*result.StaticAnalyzeResult) string {
	return sfanalysis.FormatSyntaxFlowErrors(content, errs)
}
