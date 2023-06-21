package examples

type Config struct {
	captchaUrl  string
	captchaMode string

	usernameList []string
	passwordList []string

	wsAddress string
	proxy     string
	proxyUser string
	proxyPass string

	usernameSelector     string
	passwordSelector     string
	captchaSelector      string
	captchaImgSelector   string
	submitButtonSelector string

	ch chan *BruteForceResult
}

type ConfigOpt func(*Config)

func WithCaptchaUrl(url string) ConfigOpt {
	return func(config *Config) {
		config.captchaUrl = url
	}
}

func WithCaptchaMode(modeName string) ConfigOpt {
	return func(config *Config) {
		config.captchaMode = modeName
	}
}

func WithUserNameList(usernameList []string) ConfigOpt {
	return func(config *Config) {
		for _, username := range usernameList {
			config.usernameList = append(config.usernameList, username)
		}
	}
}

func WithPassWordList(passwordList []string) ConfigOpt {
	return func(config *Config) {
		for _, password := range passwordList {
			config.passwordList = append(config.passwordList, password)
		}
	}
}

func WithWsAddress(wsAddress string) ConfigOpt {
	return func(config *Config) {
		config.wsAddress = wsAddress
	}
}

func WithProxy(proxy string) ConfigOpt {
	return func(config *Config) {
		config.proxy = proxy
	}
}

func WithProxyDetails(proxy, username, password string) ConfigOpt {
	return func(config *Config) {
		config.proxy = proxy
		config.proxyUser = username
		config.proxyPass = password
	}
}

func WithUsernameSelector(selector string) ConfigOpt {
	return func(config *Config) {
		config.usernameSelector = selector
	}
}

func WithPasswordSelector(selector string) ConfigOpt {
	return func(config *Config) {
		config.passwordSelector = selector
	}
}

func WithCaptchaSelector(selector string) ConfigOpt {
	return func(config *Config) {
		config.captchaSelector = selector
	}
}

func WithCaptchaImgSelector(selector string) ConfigOpt {
	return func(config *Config) {
		config.captchaImgSelector = selector
	}
}

func WithSubmitButtonSelector(selector string) ConfigOpt {
	return func(config *Config) {
		config.submitButtonSelector = selector
	}
}

func WithResultChannel(ch chan *BruteForceResult) ConfigOpt {
	return func(config *Config) {
		config.ch = ch
	}
}
