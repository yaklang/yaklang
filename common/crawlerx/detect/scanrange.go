package detect

import (
	"regexp"
	"strings"
	"github.com/yaklang/yaklang/common/log"
)

const AllDomain = 1
const SubMenu = 2
const TargetUrl = 3

func GetValidRangeFunc(urlStr string, rangeLevel int) func(string) bool {
	if urlStr == "" {
		log.Errorf("urlStr nil")
		return nil
	}

	var mainDomain, subMenu string
	if strings.HasPrefix(urlStr, "http") {
		r, _ := regexp.Compile("http(s??)://.+?/")
		mainDomains := r.FindAllString(urlStr, -1)
		if len(mainDomains) == 0 {
			mainDomain = urlStr
			//log.Errorf("url %s main domain not found.", urlStr)
			//return nil
		} else {
			mainDomain = mainDomains[0]
		}
	} else if strings.Contains(urlStr, "/") {
		mainDomain = strings.Split(urlStr, "/")[0]
	} else {
		mainDomain = urlStr
		subMenu = urlStr
	}

	if subMenu == "" {
		if strings.HasSuffix(urlStr, "/") {
			subMenu = urlStr
		} else {
			blocks := strings.Split(urlStr, "/")
			last := blocks[len(blocks)-1]
			subMenu = strings.Replace(urlStr, last, "", -1)
		}
	}

	if rangeLevel == AllDomain {
		return func(s string) bool {
			return strings.Contains(s, mainDomain)
		}
	} else if rangeLevel == SubMenu {
		return func(s string) bool {
			return strings.Contains(s, subMenu)
		}
	} else if rangeLevel == TargetUrl {
		return func(s string) bool {
			return urlStr == s
		}
	}
	log.Errorf("range level error")
	return func(_ string) bool {
		return true
	}
}
