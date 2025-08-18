package aiforge

type TextAnalysisResult struct {
	Text string
}

func (a TextAnalysisResult) GetCumulativeSummary() string {
	return ""
}

func (a TextAnalysisResult) Dump() string {
	return a.Text
}
