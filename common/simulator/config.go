// Package simulator
// @Author bcy2007  2023/8/17 16:18
package simulator

import (
	"net/url"
)

type LeaklessMode int

const (
	LeaklessDefault LeaklessMode = 0
	LeaklessOn      LeaklessMode = 1
	LeaklessOff     LeaklessMode = -1
)

type loginDetectMode int

const (
	UrlChangeMode     loginDetectMode = 0
	HtmlChangeMode    loginDetectMode = 1
	DefaultChangeMode loginDetectMode = -1
)

type BrowserConfig struct {
	exePath   string
	wsAddress string
	proxy     *url.URL
	leakless  LeaklessMode
}

type BrowserConfigOpt func(*BrowserConfig)

func CreateNewBrowserConfig() *BrowserConfig {
	config := BrowserConfig{
		leakless: LeaklessDefault,
	}
	return &config
}

func withExePath(exePath string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.exePath = exePath
	}
}

func withWsAddress(ws string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.wsAddress = ws
	}
}

func withProxy(proxy *url.URL) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.proxy = proxy
	}
}

func withLeakless(leakless LeaklessMode) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.leakless = leakless
	}
}

type BruteConfig struct {
	wsAddress     string
	exePath       string
	proxy         string
	proxyUsername string
	proxyPassword string

	usernameList []string
	passwordList []string

	captchaUrl  string
	captchaMode string

	usernameSelector    string
	passwordSelector    string
	captchaSelector     string
	captchaImgSelector  string
	loginButtonSelector string

	ch                chan Result
	loginDetect       loginDetectMode
	leakless          LeaklessMode
	extraWaitLoadTime int
	similarityDegree  float64
}

type BruteConfigOpt func(*BruteConfig)

func NewBruteConfig() *BruteConfig {
	return &BruteConfig{
		usernameList:     make([]string, 0),
		passwordList:     make([]string, 0),
		loginDetect:      DefaultChangeMode,
		leakless:         LeaklessDefault,
		similarityDegree: 0.6,
	}
}

func WithWsAddress(ws string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.wsAddress = ws
	}
}

func WithExePath(exePath string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.exePath = exePath
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

func WithUsername(username []string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameList = append(config.usernameList, username...)
	}
}

func WithUsernameList(username ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameList = append(config.usernameList, username...)
	}
}

func WithPassword(password []string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordList = append(config.passwordList, password...)
	}
}

func WithPasswordList(password ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordList = append(config.passwordList, password...)
	}
}

func WithCaptchaUrl(url string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaUrl = url
	}
}

func WithCaptchaMode(mode string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaMode = mode
	}
}

func WithUsernameSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameSelector = selector
	}
}

func WithPasswordSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordSelector = selector
	}
}
func WithCaptchaSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaSelector = selector
	}
}
func WithCaptchaImgSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaImgSelector = selector
	}
}
func WithLoginButtonSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.loginButtonSelector = selector
	}
}

func WithResultChannel(ch chan Result) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.ch = ch
	}
}

func WithLoginDetectMode(mode loginDetectMode, degree ...float64) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.loginDetect = mode
		if len(degree) > 0 {
			config.similarityDegree = degree[0]
		}
	}
}

func WithLeakless(leakless LeaklessMode) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.leakless = leakless
	}
}

func WithExtraWaitLoadTime(time int) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.extraWaitLoadTime = time
	}
}
