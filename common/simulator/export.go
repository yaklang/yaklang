package simulator

import (
	"yaklang/common/simulator/core"
	"yaklang/common/simulator/examples"
	"yaklang/common/simulator/extend"
	"yaklang/common/simulator/simple"
)

var Exports = map[string]interface{}{
	"Page":    core.PageCreator,
	"Captcha": extend.CreateCaptcha,

	"defaultBrute": examples.BruteForceModuleV2,

	"captchaUrl":   examples.WithCaptchaUrl,
	"captchaMode":  examples.WithCaptchaMode,
	"usernameList": examples.WithUserNameList,
	"passwordList": examples.WithPassWordList,

	"wsAddress":    examples.WithWsAddress,
	"proxy":        examples.WithProxy,
	"proxyDetails": examples.WithProxyDetails,

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
