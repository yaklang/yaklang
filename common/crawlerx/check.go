// Package crawlerx
// @Author bcy2007  2023/7/12 17:31
package crawlerx

import (
	"fmt"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

func repeatCheckGenerator(level repeatLevel, extraParams ...string) func(HijackRequest) string {
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

func extremeLevelRepeatCheckGenerator() func(HijackRequest) string {
	return func(request HijackRequest) string {
		parsed := request.URL()
		return getUrlPage(parsed)
	}
}

func highLevelRepeatCheckGenerator() func(HijackRequest) string {
	return func(request HijackRequest) string {
		parsed := request.URL()
		return request.Method() + " " + getUrlPage(parsed)
	}
}

func midLevelRepeatCheckGenerator(extraParams ...string) func(HijackRequest) string {
	checkF := doCheck(extraParams...)
	return func(request HijackRequest) string {
		parsed := request.URL()
		return request.Method() + " " + getUrlQueryName(parsed, checkF)
	}
}

func lowLevelRepeatCheckGenerator(extraParams ...string) func(HijackRequest) string {
	if len(extraParams) == 0 {
		return func(request HijackRequest) string {
			return request.Method() + " " + request.URL().String()
		}
	}
	checkF := doCheck(extraParams...)
	return func(request HijackRequest) string {
		parsed := request.URL()
		return request.Method() + " " + getUrlQueryValue(parsed, checkF)
	}
}

func unLimitLevelRepeatCheckGenerator(extraParams ...string) func(HijackRequest) string {
	if len(extraParams) == 0 {
		return func(request HijackRequest) string {
			result := request.Method() + " " + request.URL().String()
			if request.Body() != "" {
				result += " " + request.Body()
			}
			return result
		}
	}
	checkF := doCheck(extraParams...)
	return func(request HijackRequest) string {
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

var boardDomainCompilerStr = "^http(s?)://([^/]*?\\.)?%s/"

func scanRangeFunctionGenerate(targetUrl string, scanRange scanRangeLevel) func(string) bool {
	rangeFunction, ok := generalScanRangeMap[scanRange]
	if !ok {
		return nil
	}
	rangeUrl := rangeFunction(targetUrl)
	if len(rangeUrl) > 0 && !strings.HasPrefix(rangeUrl[0], "http") {
		regs := make([]*regexp.Regexp, 0, len(rangeUrl))
		for _, url := range rangeUrl {
			reg, err := regexp.Compile(fmt.Sprintf(boardDomainCompilerStr, url))
			if err != nil {
				log.Debugf("reg %s compiler error: %v", url, err)
				continue
			}
			regs = append(regs, reg)
		}
		return func(checkUrl string) bool {
			if StringSuffixList(checkUrl, extraUrlKeywords) {
				return true
			}
			for _, reg := range regs {
				if reg.MatchString(checkUrl) {
					return true
				}
			}
			return false
		}
	}
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

func generalBoardDomainRange(targetUrl string) []string {
	url, _ := u.Parse(targetUrl)
	ranges := make([]string, 0)
	items := strings.Split(url.Host, ".")
	if len(items) > 1 {
		ranges = append(ranges, fmt.Sprintf("%s.%s", items[len(items)-2], items[len(items)-1]))
	}
	return ranges
}

func generalSubDomainRange(targetUrl string) []string {
	url, _ := u.Parse(targetUrl)
	ranges := make([]string, 0)
	var path string
	if strings.HasSuffix(url.Path, "/") {
		path = url.Path
	} else {
		tempPaths := strings.Split(url.Path, "/")
		path = strings.Join(tempPaths[0:len(tempPaths)-1], "/") + "/"
	}
	ranges = append(ranges, url.Scheme+"://"+url.Host+path)
	if !strings.HasPrefix(url.Host, "www.") {
		ranges = append(ranges, url.Scheme+"://www."+url.Host+path)
	}
	return ranges
}

func generalUnlimitedDomainRange(_ string) []string {
	return []string{"*"}
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
	if url.RawQuery == "" {
		return getUrlPage(url)
	}
	query, _ := GetSortedQuery(url.RawQuery)
	queryLength := len(query)
	queryItems := make([]string, 0)
	for i := 0; i < queryLength-1; i += 2 {
		if check(query[i]) {
			continue
		}
		queryItems = append(queryItems, query[i])
	}
	return getUrlPage(url) + "?" + strings.Join(queryItems, "&")
}

func getUrlQueryValue(url *u.URL, check func(string) bool) string {
	if url.RawQuery == "" {
		return getUrlPage(url)
	}
	query, _ := GetSortedQuery(url.RawQuery)
	queryLength := len(query)
	queryItems := make([]string, 0)
	for i := 0; i < queryLength-1; i += 2 {
		if check(query[i]) {
			continue
		}
		queryItems = append(queryItems, query[i]+"="+query[i+1])
	}
	return getUrlPage(url) + "?" + strings.Join(queryItems, "&")
}

func extraUrlCheck(extraSuffix []string) func(string) bool {
	return func(urlStr string) bool {
		return StringSuffixList(urlStr, extraSuffix)
	}
}

func GetSortedQuery(rawQuery string) (query []string, err error) {
	query = make([]string, 0)
	if rawQuery == "" {
		return
	}
	for rawQuery != "" {
		var key string
		key, rawQuery, _ = strings.Cut(rawQuery, "&")
		if strings.Contains(key, ";") {
			err = utils.Error("invalid semicolon separator in query")
			continue
		}
		if key == "" {
			continue
		}
		key, value, _ := strings.Cut(key, "=")
		key, err1 := u.QueryUnescape(key)
		if err1 != nil {
			if err == nil {
				err = utils.Error(err1)
			}
			continue
		}
		value, err1 = u.QueryUnescape(value)
		if err1 != nil {
			if err == nil {
				err = utils.Error(err1)
			}
			continue
		}
		query = append(query, key, value)
	}
	return
}
