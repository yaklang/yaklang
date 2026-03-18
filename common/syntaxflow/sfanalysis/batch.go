package sfanalysis

import "context"

// BatchAnalyze evaluates multiple SyntaxFlow rules and returns their quality reports.
//
// It uses the same scoring behavior as ProfileQuality.
func BatchAnalyze(rules map[string]string) map[string]*SyntaxFlowRuleAnalyzeResult {
	if len(rules) == 0 {
		return map[string]*SyntaxFlowRuleAnalyzeResult{}
	}

	opts := DefaultOptions(ProfileQuality)
	results := make(map[string]*SyntaxFlowRuleAnalyzeResult, len(rules))
	for name, content := range rules {
		report := Analyze(context.Background(), content, opts)
		if report == nil || report.Quality == nil {
			results[name] = &SyntaxFlowRuleAnalyzeResult{Score: MinScore, MaxScore: MaxScore}
			continue
		}
		results[name] = report.Quality
	}
	return results
}
