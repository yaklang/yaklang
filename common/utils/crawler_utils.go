package utils

import (
	"fmt"
	"github.com/pkg/errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// map[string][]string to query
func MapQueryToString(values map[string][]string) string {
	var items []string
	for key, vals := range values {
		key = url.QueryEscape(key)
		for _, val := range vals {
			val = url.QueryEscape(val)

			var (
				item string
			)
			if key != "" {
				item = fmt.Sprintf("%s=%s", key, val)
			} else {
				item = val
			}

			if item != "" {
				items = append(items, item)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i] < items[j]
	})
	return strings.Join(items, "&")
}

func StarAsWildcardToRegexp(prefix string, target string) (*regexp.Regexp, error) {
	var urlFilterTmp = "%s"
	if prefix != "" {
		urlFilterTmp = "^" + prefix + "%s" // https?://
	}

	var re string
	if !strings.Contains(target, "/") {
		// 不包含路径匹配，那么通配符不应该包含路径和问号
		if !strings.Contains(target, "*") {
			re = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(target))
		} else {
			re = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(strings.ReplaceAll(target, "*", "WILDCARD")))
			re = strings.ReplaceAll(re, "WILDCARD", "[^/^?^#]*")
		}
	} else {
		results := strings.SplitN(target, "/", 2)
		if len(results) != 2 {
			return nil, errors.Errorf("[%s] split path failed", target)
		}

		domain, path := results[0], results[1]
		var (
			domainRegex, pathRegex string
		)

		// 匹配域名部分
		if !strings.Contains(domain, "*") {
			domainRegex = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(domain))
		} else {
			domainRegex = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(strings.ReplaceAll(domain, "*", "WILDCARD")))
			domainRegex = strings.ReplaceAll(domainRegex, "WILDCARD", "[^/^?^#^&]*")
		}

		// 匹配路径部分
		if strings.Contains(path, "*") {
			pathRegex = strings.ReplaceAll(path, "*", ".*")
		}

		re = fmt.Sprintf(`%s/%s`, domainRegex, pathRegex)
		//if re == "\\/" {
		//	log.Fatalf("domain: %s path: %s re: %s", domainRegex, pathRegex, re)
		//}
	}

	return regexp.Compile(re)
}

func DomainToURLFilter(domain string) (*regexp.Regexp, error) {
	return StarAsWildcardToRegexp("https?://", domain)
}
