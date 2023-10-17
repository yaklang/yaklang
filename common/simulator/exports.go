// Package simulator
// @Author bcy2007  2023/8/21 15:19
package simulator

import "github.com/yaklang/yaklang/common/simulator/simple"

var Exports = map[string]interface{}{
	"HttpBruteForce": HttpBruteForce,

	"username":             WithUsernameList,
	"usernameList":         WithUsername,
	"password":             WithPasswordList,
	"passwordList":         WithPassword,
	"wsAddress":            WithWsAddress,
	"proxy":                WithProxy,
	"captchaUrl":           WithCaptchaUrl,
	"captchaMode":          WithCaptchaMode,
	"usernameSelector":     WithUsernameSelector,
	"passwordSelector":     WithPasswordSelector,
	"captchaInputSelector": WithCaptchaSelector,
	"captchaImgSelector":   WithCaptchaImgSelector,
	"submitButtonSelector": WithLoginButtonSelector,
	"loginDetectMode":      WithLoginDetectMode,
	"exePath":              WithExePath,
	"extraWaitLoadTime":    WithExtraWaitLoadTime,
	"leaklessStatus":       WithLeakless,

	"urlChangeMode":     UrlChangeMode,
	"htmlChangeMode":    HtmlChangeMode,
	"defaultChangeMode": DefaultChangeMode,

	"leaklessDefault": LeaklessDefault,
	"leaklessOn":      LeaklessOn,
	"leaklessOff":     LeaklessOff,

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
