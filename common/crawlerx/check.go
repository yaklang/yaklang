// Package crawlerx
// @Author bcy2007  2023/7/12 17:31
package crawlerx

import (
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	u "net/url"
	"regexp"
	"strings"
)

func repeatCheckFunctionGenerate(pageVisitFilter *tools.StringCountFilter) func(string) bool {
	return func(urlStr string) bool {
		if urlStr == "" {
			return false
		}
		sha256Url := codec.Sha256(urlStr)
		if pageVisitFilter.Exist(sha256Url) {
			return false
		}
		pageVisitFilter.Insert(sha256Url)
		return true
	}
}

func repeatCheckGenerator(level repeatLevel, extraParams ...string) func(*rod.HijackRequest) string {
	switch level {
	case extremeLevel:
		return extremeLevelRepeatCheckGenerator()
	case highLevel:
		return highLevelRepeatCheckGenerator()
	case midLevel:
		return midLevelRepeatCheckGenerator(extraParams...)
	case lowLevel:
		return lowLevelRepeatCheckGenerator(extraParams...)
	case unlimited:
		return unLimitLevelRepeatCheckGenerator(extraParams...)
	}
	return nil
}

func urlRepeatCheckGenerator(level repeatLevel, extraParams ...string) func(string) string {
	checkF := doCheck(extraParams...)
	switch level {
	case extremeLevel, highLevel:
		return func(origin string) string {
			url, err := u.Parse(origin)
			if err != nil {
				log.Errorf("url %s parse error: %s", origin, err)
				return ""
			}
			return getUrlPage(url)
		}
	case midLevel:
		return func(origin string) string {
			url, err := u.Parse(origin)
			if err != nil {
				log.Errorf("url %s parse error: %s", origin, err)
				return ""
			}
			return getUrlQueryName(url, checkF)
		}
	case lowLevel, unlimited:
		return func(origin string) string {
			url, err := u.Parse(origin)
			if err != nil {
				log.Errorf("url %s parse error: %s", origin, err)
				return ""
			}
			return getUrlQueryValue(url, checkF)
		}
	}
	return func(_ string) string {
		return ""
	}
}

func extremeLevelRepeatCheckGenerator() func(*rod.HijackRequest) string {
	return func(request *rod.HijackRequest) string {
		parsed := request.URL()
		return getUrlPage(parsed)
	}
}

func highLevelRepeatCheckGenerator() func(*rod.HijackRequest) string {
	return func(request *rod.HijackRequest) string {
		parsed := request.URL()
		return request.Method() + " " + getUrlPage(parsed)
	}
}

func midLevelRepeatCheckGenerator(extraParams ...string) func(*rod.HijackRequest) string {
	checkF := doCheck(extraParams...)
	return func(request *rod.HijackRequest) string {
		parsed := request.URL()
		return request.Method() + " " + getUrlQueryName(parsed, checkF)
	}
}

func lowLevelRepeatCheckGenerator(extraParams ...string) func(*rod.HijackRequest) string {
	if len(extraParams) == 0 {
		return func(request *rod.HijackRequest) string {
			return request.Method() + " " + request.URL().String()
		}
	}
	checkF := doCheck(extraParams...)
	return func(request *rod.HijackRequest) string {
		parsed := request.URL()
		return request.Method() + " " + getUrlQueryValue(parsed, checkF)
	}
}

func unLimitLevelRepeatCheckGenerator(extraParams ...string) func(*rod.HijackRequest) string {
	if len(extraParams) == 0 {
		return func(request *rod.HijackRequest) string {
			result := request.Method() + " " + request.URL().String()
			if request.Body() != "" {
				result += " " + request.Body()
			}
			return result
		}
	}
	checkF := doCheck(extraParams...)
	return func(request *rod.HijackRequest) string {
		parsed := request.URL()
		result := request.Method() + " " + getUrlQueryValue(parsed, checkF)
		if request.Body() != "" {
			result += " " + request.Body()
		}
		return result
	}
}

//
// scan range
//

func scanRangeFunctionGenerate(targetUrl string, scanRange scanRangeLevel) func(string) bool {
	rangeFunction, ok := generalScanRangeMap[scanRange]
	if !ok {
		return nil
	}
	rangeUrl := rangeFunction(targetUrl)
	return func(checkUrl string) bool {
		if StringSuffixList(checkUrl, extraUrlKeywords) {
			return true
		}
		if StringPrefixList(checkUrl, rangeUrl) {
			return true
		}
		return false
	}
}

func generalMainDomainRange(targetUrl string) []string {
	url, _ := u.Parse(targetUrl)
	ranges := make([]string, 0)
	ranges = append(ranges, url.Scheme+"://"+url.Host)
	if !strings.HasPrefix(url.Host, "www.") {
		ranges = append(ranges, url.Scheme+"://www."+url.Host)
	}
	return ranges
}

func generalSubDomainRange(targetUrl string) []string {
	url, _ := u.Parse(targetUrl)
	ranges := make([]string, 0)
	ranges = append(ranges, url.Scheme+"://"+url.Host+url.Path)
	if !strings.HasPrefix(url.Host, "www.") {
		ranges = append(ranges, url.Scheme+"://www."+url.Host+url.Path)
	}
	return ranges
}

func whiteListCheckGenerator(whitelist []*regexp.Regexp) func(string) bool {
	return func(url string) bool {
		for _, whiteReg := range whitelist {
			if whiteReg.MatchString(url) {
				return true
			}
		}
		return false
	}
}

func blackListCheckGenerator(blacklist []*regexp.Regexp) func(string) bool {
	return func(url string) bool {
		for _, blackReg := range blacklist {
			if blackReg.MatchString(url) {
				return false
			}
		}
		return true
	}
}

func doCheck(doCheck ...string) func(k string) bool {
	if len(doCheck) == 0 {
		return func(k string) bool {
			return false
		}
	}
	return func(k string) bool {
		return funk.Contains(doCheck, k)
	}
}

func getUrlPage(url *u.URL) string {
	urlStr := url.Scheme + "://" + url.Host + url.Path
	if url.Fragment != "" {
		urlStr += "#" + url.Fragment
	}
	return urlStr
}

func getUrlQueryName(url *u.URL, check func(string) bool) string {
	query := url.Query()
	var queryStr string
	for k := range query {
		if check(k) {
			continue
		}
		queryStr += k + "&"
	}
	if queryStr != "" {
		queryStr = "?" + queryStr[:len(queryStr)-1]
	}
	return getUrlPage(url) + queryStr
}

func getUrlQueryValue(url *u.URL, check func(string) bool) string {
	query := url.Query()
	var queryStr string
	for k, v := range query {
		if check(k) {
			continue
		}
		queryStr += k + "=" + v[0] + "&"
	}
	if queryStr != "" {
		queryStr = "?" + queryStr[:len(queryStr)-1]
	}
	return getUrlPage(url) + queryStr
}

func extraUrlCheck(extraSuffix []string) func(string) bool {
	return func(urlStr string) bool {
		return StringSuffixList(urlStr, extraSuffix)
	}
}
