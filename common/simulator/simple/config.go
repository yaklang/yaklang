package simple

type BrowserConfig struct {
	wsAddress     string
	exePath       string
	proxyAddress  string
	proxyUsername string
	proxyPassword string

	noSandBox bool
	headless  bool
	hijack    bool

	runtimeID  string
	fromPlugin string
	sourceType string
	saveToDB   bool

	responseModification []*ResponseModification
	requestModification  []*RequestModification
}

type BrowserConfigOpt func(*BrowserConfig)

func WithWsAddress(wsAddress string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.wsAddress = wsAddress
	}
}

func WithExePath(exePath string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.exePath = exePath
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

func WithHijack(hijack bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.hijack = hijack
	}
}

func WithRuntimeID(runtimeID string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.runtimeID = runtimeID
	}
}

func WithFromPlugin(fromPlugin string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.fromPlugin = fromPlugin
	}
}

func WithSaveToDB(saveToDB bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.saveToDB = saveToDB
	}
}

func WithSourceType(sourceType string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.sourceType = sourceType
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
