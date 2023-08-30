// Package crawlerx
// @Author bcy2007  2023/7/12 16:56
package crawlerx

var CrawlerXExports = map[string]interface{}{
	"StartCrawler": StartCrawler,

	"browserInfo":       WithBrowserInfo,
	"saveToDB":          WithSaveToDB,
	"runtimeId":         WithRuntimeID,
	"maxUrl":            WithMaxUrl,
	"maxDepth":          WithMaxDepth,
	"concurrent":        WithConcurrent,
	"blacklist":         WithBlackList,
	"whitelist":         WithWhiteList,
	"pageTimeout":       WithPageTimeout,
	"fullTimeout":       WithFullTimeout,
	"extraWaitLoadTime": WithExtraWaitLoadTime,
	"formFill":          WithFormFill,
	"fileInput":         WithFileInput,
	"headers":           WithHeaders,
	"rawHeaders":        WithHeaderInfo,
	"cookies":           WithCookies,
	"rawCookie":         WithCookieInfo,
	"scanRangeLevel":    WithScanRangeLevel,
	"scanRepeatLevel":   WithScanRepeatLevel,
	"ignoreQueryName":   WithIgnoreQueryName,
	"sensitiveWords":    WithSensitiveWords,
	"leakless":          WithLeakless,
	"localStorage":      WithLocalStorage,
	"invalidSuffix":     WithInvalidSuffix,
	"stealth":           WithStealth,
	"runtimeID":         WithRuntimeID,

	"UnLimitRepeat":      unlimited,
	"LowRepeatLevel":     lowLevel,
	"MediumRepeatLevel":  midLevel,
	"HighRepeatLevel":    highLevel,
	"ExtremeRepeatLevel": extremeLevel,

	"AllDomainScan": mainDomain,
	"SubMenuScan":   subDomain,
}
