package mutate

import (
	"github.com/gobwas/glob"
	"strconv"
)

var cookieKeyWhiteList = [...]glob.Glob{
	glob.MustCompile("PHPSESSID"),
	glob.MustCompile("JSESSIONID"),
	glob.MustCompile("Hm_*"),
	glob.MustCompile("_ga"),
	glob.MustCompile("_gid"),
	glob.MustCompile("*_gtag_*"),
	glob.MustCompile("__utm?"),
	glob.MustCompile("aliyungf_tc"),
	glob.MustCompile("UM_*"),
	glob.MustCompile("CNZZDATA*"), // cnzz
	glob.MustCompile("__cfduid"),  // cf
	glob.MustCompile(`_gh_sess`),  // gh
}

func shouldIgnoreCookie(cookieKeyName string) bool {
	for _, i := range cookieKeyWhiteList {
		if i.Match(cookieKeyName) {
			return true
		}
	}
	return false
}

func strVisible(key string) bool {
	for _, c := range key {
		if !strconv.IsPrint(c) {
			return false
		}
	}
	return true
}
