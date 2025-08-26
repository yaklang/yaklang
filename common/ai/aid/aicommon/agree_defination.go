package aicommon

type AgreePolicyType string

const (
	AgreePolicyYOLO AgreePolicyType = "yolo"
	// auto: auto agree, should with interval at least 10 seconds
	AgreePolicyAuto AgreePolicyType = "auto"
	// manual: block until user agree
	AgreePolicyManual AgreePolicyType = "manual"
	// ai: use ai to agree, is ai is not agree, will use manual
	AgreePolicyAI AgreePolicyType = "ai"
	// ai-auto: use ai to agree, if ai is not agree, will use auto in auto interval
	AgreePolicyAIAuto AgreePolicyType = "ai-auto"
)
