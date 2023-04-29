package crawler

import (
	"yaklang/common/log"
	"yaklang/common/utils"
)

var Exports = map[string]interface{}{
	"Start": func(url string, opt ...configOpt) (chan *Req, error) {
		ch := make(chan *Req)
		opt = append(opt, WithOnRequest(func(req *Req) {
			ch <- req
		}))

		crawler, err := NewCrawler(url, opt...)
		if err != nil {
			return nil, utils.Errorf("create crawler failed: %s", err)
		}
		go func() {
			defer close(ch)

			err := crawler.Run()
			if err != nil {
				log.Error(err)
			}
		}()
		return ch, nil
	},

	"basicAuth":           WithBasicAuth,
	"bodySize":            WithBodySize,
	"concurrent":          WithConcurrent,
	"connectTimeout":      WithConnectTimeout,
	"timeout":             WithConnectTimeout,
	"domainExclude":       WithDomainBlackList,
	"domainInclude":       WithDomainWhiteList,
	"cookie":              WithFixedCookie,
	"forbiddenFromParent": WithForbiddenFromParent,
	"disallowSuffix":      WithDisallowSuffix,
	"header":              WithHeader,
	"urlExtractor":        WithUrlExtractor,
	"maxDepth":            WithMaxDepth,
	"maxRedirect":         WithMaxRedirectTimes,
	"maxRequest":          WithMaxRequestCount,
	"maxRetry":            WithMaxRetry,
	"maxUrls":             WithMaxUrlCount,
	"proxy":               WithProxy,
	"responseTimeout":     WithResponseTimeout,
	"urlRegexpExclude":    WithUrlRegexpBlackList,
	"urlRegexpInclude":    WithUrlRegexpWhiteList,
	"userAgent":           WithUserAgent,
	"ua":                  WithUserAgent,
	"autoLogin":           WithAutoLogin,
	"RequestsFromFlow":    HandleRequestResult,
}
