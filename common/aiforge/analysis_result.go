package aiforge

type AnalysisResult interface {
	GetCumulativeSummary() string
	Dump() string
}
