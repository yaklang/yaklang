package simple

type BrowserConfig struct {
	wsAddress     string
	proxyAddress  string
	proxyUsername string
	proxyPassword string

	noSandBox bool
	headless  bool

	responseModification []*ResponseModification
	requestModification  []*RequestModification
}

type BrowserConfigOpt func(*BrowserConfig)

func WithWsAddress(wsAddress string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.wsAddress = wsAddress
	}
}

func WithProxy(proxyAddress string, proxyUserInfo ...string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.proxyAddress = proxyAddress
		if len(proxyUserInfo) > 1 {
			config.proxyUsername = proxyUserInfo[0]
			config.proxyPassword = proxyUserInfo[1]
		}
	}
}

func WithNoSandBox(noSandBox bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.noSandBox = noSandBox
	}
}

func WithHeadless(headless bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.headless = headless
	}
}

func WithResponseModification(modifyUrl string, modifyTarget ModifyTarget, modifyResult interface{}) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.responseModification = append(config.responseModification, &ResponseModification{
			baseModify{modifyUrl: modifyUrl, modifyTarget: modifyTarget, modifyResult: modifyResult},
		})
	}
}

func WithRequestModification(modifyUrl string, modifyTarget ModifyTarget, modifyResult interface{}) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.requestModification = append(config.requestModification, &RequestModification{
			baseModify{modifyUrl: modifyUrl, modifyTarget: modifyTarget, modifyResult: modifyResult},
		})
	}
}
