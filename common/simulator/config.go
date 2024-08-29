// Package simulator
// @Author bcy2007  2023/8/17 16:18
package simulator

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/crawlerx/preaction"
	"github.com/yaklang/yaklang/common/log"
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

	saveToDB   bool
	sourceType string
	fromPlugin string
	runtimeID  string
}

var actionDict = map[string]preaction.ActionType{
	"hover":   preaction.HoverAction,
	"click":   preaction.ClickAction,
	"input":   preaction.InputAction,
	"select":  preaction.SelectAction,
	"setFile": preaction.SetFileAction,
}

type BrowserConfigOpt func(*BrowserConfig)

func CreateNewBrowserConfig() *BrowserConfig {
	config := BrowserConfig{
		leakless:   LeaklessDefault,
		saveToDB:   false,
		sourceType: "scan",
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

func withSaveToDB(saveToDB bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.saveToDB = saveToDB
	}
}

func withSourceType(sourceType string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.sourceType = sourceType
	}
}

func withFromPlugin(fromPlugin string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.fromPlugin = fromPlugin
	}
}

func withRuntimeID(runtimeID string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.runtimeID = runtimeID
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
	captchaType int

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

	saveToDB   bool
	sourceType string
	fromPlugin string
	runtimeID  string

	preActions []*preaction.PreAction
}

type BruteConfigOpt func(*BruteConfig)

var NullBruteConfigOpt = func(*BruteConfig) {}

func NewBruteConfig() *BruteConfig {
	return &BruteConfig{
		usernameList:     make([]string, 0),
		passwordList:     make([]string, 0),
		loginDetect:      DefaultChangeMode,
		leakless:         LeaklessDefault,
		similarityDegree: 0.6,

		saveToDB:   false,
		sourceType: "scan",
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

func WithCaptchaType(typeEnum int) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaType = typeEnum
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

func WithSaveToDB(saveToDB bool) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.saveToDB = saveToDB
	}
}

func WithSourceType(sourceType string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.sourceType = sourceType
	}
}

func WithFromPlugin(fromPlugin string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.fromPlugin = fromPlugin
	}
}

func WithRuntimeID(runtimeID string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.runtimeID = runtimeID
	}
}

func WithPreActions(actionsJs string) BruteConfigOpt {
	var actions []preaction.PreActionJson
	err := json.Unmarshal([]byte(actionsJs), &actions)
	if err != nil {
		log.Errorf("unmarshal preaction json string error: %v", err)
		return NullBruteConfigOpt
	}
	return func(config *BruteConfig) {
		tempActions := make([]*preaction.PreAction, 0)
		for _, action := range actions {
			actionType, ok := actionDict[action.Action]
			if !ok {
				log.Errorf("invalid action string: %v", action.Action)
				continue
			}
			tempActions = append(tempActions, &preaction.PreAction{
				Action:   actionType,
				Selector: action.Selector,
				Params:   action.Params,
			})
		}
		config.preActions = tempActions
	}
}
