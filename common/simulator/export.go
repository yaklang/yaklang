package simulator

import (
	"github.com/yaklang/yaklang/common/simulator/examples"
	"github.com/yaklang/yaklang/common/simulator/simple"
)

var Exports = map[string]interface{}{
	//"Page":    core.PageCreator,
	//"Captcha": extend.CreateCaptcha,

	"defaultBrute": examples.BruteForceModuleV2,

	"captchaUrl":   examples.WithCaptchaUrl,
	"captchaMode":  examples.WithCaptchaMode,
	"usernameList": examples.WithUserNameList,
	"passwordList": examples.WithPassWordList,

	"wsAddress":    examples.WithWsAddress,
	"proxy":        examples.WithProxy,
	"proxyDetails": examples.WithProxyDetails,

	"usernameSelector":     examples.WithUsernameSelector,
	"passwordSelector":     examples.WithPasswordSelector,
	"captchaInputSelector": examples.WithCaptchaSelector,
	"captchaImgSelector":   examples.WithCaptchaImgSelector,
	"submitButtonSelector": examples.WithSubmitButtonSelector,

	"simple": SimpleExports,
}

var SimpleExports = map[string]interface{}{
	"createBrowser": simple.CreateHeadlessBrowser,

	"wsAddress":      simple.WithWsAddress,
	"proxy":          simple.WithProxy,
	"noSandBox":      simple.WithNoSandBox,
	"headless":       simple.WithHeadless,
	"requestModify":  simple.WithRequestModification,
	"responseModify": simple.WithResponseModification,

	"bodyModifyTarget":    simple.BodyModifyTarget,
	"bodyReplaceTarget":   simple.BodyReplaceTarget,
	"headersModifyTarget": simple.HeadersModifyTarget,
	"hostModifyTarget":    simple.HostModifyTarget,
}
