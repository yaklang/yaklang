// Package simulator
// @Author bcy2007  2023/8/21 15:19
package simulator

var exports = map[string]interface{}{
	"HttpBruteForce":       HttpBruteForce,
	"exePath":              WithExePath,
	"wsAddress":            WithWsAddress,
	"proxy":                WithProxy,
	"username":             WithUsernameList,
	"password":             WithPasswordList,
	"captchaUrl":           WithCaptchaUrl,
	"captchaMode":          WithCaptchaMode,
	"usernameSelector":     WithUsernameSelector,
	"passwordSelector":     WithPasswordSelector,
	"captchaInputSelector": WithCaptchaSelector,
	"captchaImgSelector":   WithCaptchaImgSelector,
	"submitButtonSelector": WithLoginButtonSelector,
	"loginDetectMode":      WithLoginDetectMode,
	"leaklessStatus":       WithLeakless,
	"extraWaitLoadTime":    WithExtraWaitLoadTime,
}

var tempExports = map[string]interface{}{
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
}
