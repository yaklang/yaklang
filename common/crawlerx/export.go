package crawlerx

import (
	"yaklang/common/crawlerx/core"
	"yaklang/common/crawlerx/detect"
)

var CrawlerXExports = map[string]interface{}{
	//"CreateCrawler": CreateCrawler,
	"StartCrawler":   StartCrawler,
	"StartCrawlerV2": StartCrawlerV2,
	"PageScreenShot": PageScreenShot,

	"proxy":        core.WithProxy,
	"maxUrl":       core.WithMaxUrl,
	"whiteList":    core.WithWhiteList,
	"blackList":    core.WithBlackList,
	"timeout":      core.WithTimeout,
	"maxDepth":     core.WithMaxDepth,
	"formFill":     core.WithFormFill,
	"header":       core.WithHeader,
	"headers":      core.WithHeaders,
	"concurrent":   core.WithConcurrent,
	"cookie":       core.WithCookie,
	"cookies":      core.WithCookies,
	"scanRange":    core.WithScanRange,
	"scanRepeat":   core.WithScanRepeat,
	"checkDanger":  core.WithCheckDanger,
	"tags":         core.WithTags,
	"fullTimeout":  core.WithFullCrawlerTimeout,
	"chromeWS":     core.WithChromeWS,
	"remote":       core.WithGetUrlRemote,
	"extraHeaders": core.WithExtraHeaders,

	"HighRepeatLevel":   detect.HighLevel,
	"MediumRepeatLevel": detect.MediumLevel,
	"LowRepeatLevel":    detect.LowLevel,
	"UnLimitRepeat":     detect.UnLimit,

	"AllDomainScan": detect.AllDomain,
	"SubMenuScan":   detect.SubMenu,
	"TargetUrlScan": detect.TargetUrl,
}
