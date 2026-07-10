package notify

type OnboardingResult struct {
	AppID     string
	AppSecret string
	Platform  PlatformType
	OwnerID   string
}

type OnboardingStep struct {
	State   string
	QrURL   string
	QrPNG   []byte
	Message string
	Result  *OnboardingResult
}

type OnboardingHandler func(step *OnboardingStep) error
