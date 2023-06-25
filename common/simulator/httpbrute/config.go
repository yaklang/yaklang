// Package httpbrute
// @Author bcy2007  2023/6/20 14:54
package httpbrute

type BruteConfig struct {
	usernameList []string
	passwordList []string

	wsAddress     string
	proxy         string
	proxyUsername string
	proxyPassword string

	captchaUrl  string
	captchaMode string

	usernameSelector   string
	passwordSelector   string
	captchaSelector    string
	captchaImgSelector string
	buttonSelector     string

	resultChannel chan Result

	loginDetect      loginDetectMode
	similarityDegree float64
}

type BruteConfigOpt func(*BruteConfig)

func NewBruteConfig() *BruteConfig {
	return &BruteConfig{
		usernameList:       make([]string, 0),
		passwordList:       make([]string, 0),
		wsAddress:          "",
		proxy:              "",
		proxyUsername:      "",
		proxyPassword:      "",
		captchaUrl:         "",
		captchaMode:        "",
		usernameSelector:   "",
		passwordSelector:   "",
		captchaSelector:    "",
		captchaImgSelector: "",
		buttonSelector:     "",
		resultChannel:      nil,
		loginDetect:        DefaultChangeMode,
		similarityDegree:   0.6,
	}
}

func WithUsername(username ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameList = append(config.usernameList, username...)
	}
}

func WithUsernames(usernames []string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameList = append(config.usernameList, usernames...)
	}
}

func WithPassword(password ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordList = append(config.passwordList, password...)
	}
}

func WithPasswords(passwords []string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordList = append(config.passwordList, passwords...)
	}
}

func WithWsAddress(wsAddress string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.wsAddress = wsAddress
	}
}

func WithProxy(proxy string, details ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.proxy = proxy
		if len(details) > 1 {
			config.proxyUsername = details[0]
			config.proxyPassword = details[1]
		}
	}
}

func WithCaptchaUrl(captchaUrl string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaUrl = captchaUrl
	}
}

func WithCaptchaMode(captchaMode string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaMode = captchaMode
	}
}

func WithUsernameSelector(usernameSelector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameSelector = usernameSelector
	}
}

func WithPasswordSelector(passwordSelector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordSelector = passwordSelector
	}
}

func WithCaptchaSelector(captchaSelector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaSelector = captchaSelector
	}
}

func WithCaptchaImgSelector(captchaImgSelector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaImgSelector = captchaImgSelector
	}
}

func WithButtonSelector(buttonSelector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.buttonSelector = buttonSelector
	}
}

func WithResultChannel(ch chan Result) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.resultChannel = ch
	}
}

func WithLoginDetectMode(detectMode loginDetectMode, degree ...float64) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.loginDetect = detectMode
		if len(degree) > 0 {
			config.similarityDegree = degree[0]
		}
	}
}
