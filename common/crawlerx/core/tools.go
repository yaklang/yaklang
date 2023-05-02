package core

import (
	"github.com/go-rod/rod"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"yaklang/common/utils"
)

func (crawler *CrawlerX) Visible(element *rod.Element) bool {
	elementType, _ := element.Attribute("type")
	if elementType != nil && *elementType == "hidden" {
		return false
	}
	elementStyle, _ := element.Attribute("style")
	if elementStyle == nil || *elementType == "" {
		return true
	}
	reg := regexp.MustCompile("\\s+")
	if strings.Contains(reg.ReplaceAllString(*elementStyle, ""), "display:none") {
		return false
	}
	return true
}

func (crawler *CrawlerX) NewVisible(element *rod.Element) bool {
	value, err := element.Visible()
	//element.Visible()
	if err != nil {
		return false
	}
	return value
}

func (crawler *CrawlerX) GetElementsSelectors(elements rod.Elements) []string {
	selectors := make([]string, 0)
	for _, element := range elements {
		selector := crawler.GetElementSelector(element)
		if selector != "" {
			selectors = append(selectors, selector)
		}
	}
	return selectors
}

func (crawler *CrawlerX) GetElementSelector(element *rod.Element) string {
	selector, err := element.Eval(getSelector)
	if err != nil {
		return ""
	}
	return selector.Value.Str()
}

func (crawler *CrawlerX) CheckValidSuffix(urlStr string) bool {
	uIns, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(uIns.EscapedPath()))
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	for _, suf := range InvalidSuffix {
		if ext == suf {
			return false
		}
	}
	return true
}

func (crawler *CrawlerX) CheckValidHost(urlStr string) bool {
	//return true
	if strings.HasPrefix(urlStr, "data:image") || strings.HasPrefix(urlStr, "javascript") {
		return false
	}
	if crawler.whiteList != nil {
		for _, whiteReg := range crawler.whiteList {
			if whiteReg.MatchString(urlStr) {
				return true
			}
		}
		return false
	}
	if crawler.blackList != nil {
		for _, blackReg := range crawler.blackList {
			if blackReg.MatchString(urlStr) {
				return false
			}
		}
		return true
	}
	return true
}

func SliceMinus(origin, target []string) []string {
	result := make([]string, 0)
	for _, item := range origin {
		if !utils.StringArrayContains(target, item) {
			result = append(result, item)
		}
	}
	return result
}

func GetAllKeyWords(element *rod.Element) string {
	var keywords string
	for _, attr := range ElementAttribute {
		attribute, _ := element.Attribute(attr)
		if attribute == nil {
			continue
		}
		keywords += *attribute + ";"
	}
	return keywords
}

func CrawlerErrorf(origin string, args ...interface{}) error {
	if strings.Contains(origin, "context canceled") {
		return nil
	}
	return utils.Errorf(origin, args)
}

func (crawler *CrawlerX) SubmitCutUrl(urlStr string) []string {
	uins, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}
	if uins.Path == "/" || uins.Path == "" {
		return nil
	}
	var path string
	if strings.HasSuffix(uins.Path, "/") {
		path = uins.Path[:len(uins.Path)-1]
	} else {
		path = uins.Path
	}
	paths := strings.Split(path, "/")
	//for num, path := range paths {
	//}
	lenPaths := len(paths)
	results := make([]string, 0)
	for i := 1; i < lenPaths-1; i++ {
		result := uins.Scheme + "://" + uins.Host + strings.Join(paths[:i+1], "/") + "/"
		results = append(results, result)
	}
	return results
}
