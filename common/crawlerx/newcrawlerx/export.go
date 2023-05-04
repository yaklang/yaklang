// Package newcrawlerx
// @Author bcy2007  2023/3/23 14:06
package newcrawlerx

var NewCrawlerXExports = map[string]interface{}{
	"startCrawler": StartCrawler,

	"browserInfo": WithNewBrowser,
	"blackList":   WithBlackList,
	"whiteList":   WithWhiteList,
	"timeout":     WithPageTimeout,
	"fullTimeout": WithTimeout,
	"formFill":    WithFormFill,
	"fileInput":   WithFileInput,
	"header":      WithHeader,
	"headers":     WithHeaders,
	"cookie":      WithCookie,
	"cookies":     WithCookies,
	"scanRange":   WithScanRangeLevel,
	"scanRepeat":  WithScanRepeatLevel,

	"maxUrl":      WithMaxUrl,
	"ignoreQuery": WithIgnoreQueryName,

	"UnLimitRepeat":      unlimited,
	"LowRepeatLevel":     lowLevel,
	"MediumRepeatLevel":  midLevel,
	"HighRepeatLevel":    highLevel,
	"ExtremeRepeatLevel": extremeLevel,

	"AllDomainScan": mainDomain,
	"SubMenuScan":   subDomain,

	"vueWebsite": WithVueWeb,
}
