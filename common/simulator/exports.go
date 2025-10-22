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
	"captchaType":          WithCaptchaType,
	"usernameSelector":     WithUsernameSelector,
	"passwordSelector":     WithPasswordSelector,
	"captchaInputSelector": WithCaptchaSelector,
	"captchaImgSelector":   WithCaptchaImgSelector,
	"submitButtonSelector": WithLoginButtonSelector,
	"loginDetectMode":      WithLoginDetectMode,
	"successMatchers":      WithSuccessMatchers,
	"exePath":              WithExePath,
	"extraWaitLoadTime":    WithExtraWaitLoadTime,
	"leaklessStatus":       WithLeakless,
	"preAction":            WithPreActions,

	"urlChangeMode":     UrlChangeMode,
	"htmlChangeMode":    HtmlChangeMode,
	"stringMatchMode":   StringMatchMode,
	"defaultChangeMode": DefaultChangeMode,

	"leaklessDefault": LeaklessDefault,
	"leaklessOn":      LeaklessOn,
	"leaklessOff":     LeaklessOff,

	"saveToDB":   WithSaveToDB,
	"sourceType": WithSourceType,
	"fromPlugin": WithFromPlugin,
	"runtimeID":  WithRuntimeID,

	"simple": SimpleExports,
}

// simulator.simple 浏览器手动操作模式
var SimpleExports = map[string]interface{}{
	"CreateBrowser": simple.CreateHeadlessBrowser,
	"createBrowser": simple.CreateHeadlessBrowser,

	"wsAddress":      simple.WithWsAddress,
	"exePath":        simple.WithExePath,
	"proxy":          simple.WithProxy,
	"noSandBox":      simple.WithNoSandBox,
	"headless":       simple.WithHeadless,
	"hijack":         simple.WithHijack,
	"timeout":        simple.WithTimeout,
	"leakless":       simple.WithLeakless,
	"requestModify":  simple.WithRequestModification,
	"responseModify": simple.WithResponseModification,

	"bodyModifyTarget":    simple.BodyModifyTarget,
	"bodyReplaceTarget":   simple.BodyReplaceTarget,
	"headersModifyTarget": simple.HeadersModifyTarget,
	"hostModifyTarget":    simple.HostModifyTarget,
}
