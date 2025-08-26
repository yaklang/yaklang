package aiforge

type AnalysisResult interface {
	GetCumulativeSummary() string
	Dump() string
}

type TextAnalysisResult struct {
	Text string
}

func (a TextAnalysisResult) GetCumulativeSummary() string {
	return ""
}

func (a TextAnalysisResult) Dump() string {
	return a.Text
}
