// Package crawlerx
// @Author bcy2007  2023/7/12 17:40
package crawlerx

import (
	"github.com/go-rod/rod/lib/proto"
	"regexp"
	"strings"

	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func getAttribute(element *rod.Element, attribute string) (string, error) {
	attributeStr, err := element.Attribute(attribute)
	if err != nil {
		return "", utils.Errorf("element %s get attribute %s error: %s", element, attribute, err)
	}
	if attributeStr == nil {
		return "", nil
	}
	return *attributeStr, nil
}

func getProperty(element *rod.Element, property string) (string, error) {
	propertyObj, err := element.Property(property)
	if err != nil {
		return "", utils.Errorf("element %s get property %s error: %s", element, property, err)
	}
	return propertyObj.String(), nil
}

func getCurrentUrl(page *rod.Page) (string, error) {
	result, err := page.Eval(`()=>document.URL`)
	if err != nil {
		return "", utils.Errorf("page %s get url error: %s", page, err)
	}
	urlStr := result.Value.Str()
	if strings.HasSuffix(urlStr, "#") {
		urlStr = urlStr[:len(urlStr)-1]
	}
	return urlStr, nil
}

func isVisible(element *rod.Element) (bool, error) {
	visible, err := element.Visible()
	if err != nil {
		return false, utils.Errorf("element %s get visiable error: %s", element, err)
	}
	return visible, nil
}

func getAllKeywords(element *rod.Element) string {
	var keywords string
	for _, attr := range elementAttribute {
		attribute, _ := getAttribute(element, attr)
		if attribute == "" {
			continue
		}
		keywords += attribute + ";"
	}
	return keywords
}

func calculateSelector(element *rod.Element) (string, error) {
	selectorObj, err := element.Eval(getSelector)
	if err != nil {
		return "", utils.Errorf("calculate selector error: %v", err)
	}
	return selectorObj.Value.Str(), nil
}

func getElementSelector(element *rod.Element) string {
	if visible, _ := element.Visible(); !visible {
		return ""
	}
	selector, err := calculateSelector(element)
	if err != nil {
		log.Error(err)
		return ""
	}
	return selector
}

func getElementsSelectors(elements rod.Elements) []string {
	selectors := make([]string, 0)
	for _, element := range elements {
		selector := getElementSelector(element)
		if selector == "" {
			continue
		}
		selectors = append(selectors, selector)
	}
	return selectors
}

func StringArrayContains(array []string, element string) bool {
	for _, s := range array {
		if element == s {
			return true
		}
	}
	return false
}

func StringArrayCover(array []string, element string) (bool, string) {
	for _, s := range array {
		if s == "" {
			continue
		}
		if strings.Contains(strings.ToLower(element), s) {
			return true, s
		}
	}
	return false, ""
}

func isSimilarSelector(s1, s2 string) bool {
	if s1 == "" || s2 == "" {
		return false
	}
	sectionsA := strings.Split(s1, ">")
	sectionsB := strings.Split(s2, ">")
	if len(sectionsA) != len(sectionsB) {
		return false
	}
	flag := true
	length := len(sectionsA)
	for i := 0; i < length; i++ {
		if sectionsA[i] != sectionsB[i] {
			if !subCheck(sectionsA[i], sectionsB[i]) {
				return false
			}
			if flag == true {
				flag = false
			} else {
				return false
			}
		}
	}
	return true
}

func subCheck(s1, s2 string) bool {
	if s1 == "" || s2 == "" {
		return false
	}
	r, _ := regexp.Compile(`(\D+)?`)
	sectionA := r.FindAllString(s1, -1)
	sectionB := r.FindAllString(s2, -1)
	if len(sectionA) != len(sectionB) {
		return false
	}
	for i := 0; i < len(sectionA); i++ {
		if sectionA[i] != sectionB[i] {
			return false
		}
	}
	return true
}

func StringSuffixList(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func StringPrefixList(origin string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "*" {
			return true
		}
		if strings.HasPrefix(origin, prefix) {
			return true
		}
	}
	return false
}

func headerRawDataTransfer(headerRawData string) []*headers {
	transferred := make([]*headers, 0)
	rowsData := strings.Split(headerRawData, "\n")
	for _, rowData := range rowsData {
		if rowData == "" {
			continue
		}
		if !strings.Contains(rowData, ": ") {
			continue
		}
		items := strings.Split(rowData, ": ")
		transferred = append(transferred, &headers{Key: items[0], Value: items[1]})
	}
	return transferred
}

func cookieRawDataTransfer(domain, cookieRawData string) []*proto.NetworkCookieParam {
	transferred := make([]*proto.NetworkCookieParam, 0)
	if strings.HasPrefix(cookieRawData, "Cookie: ") {
		cookieRawData = strings.Replace(cookieRawData, "Cookie: ", "", 1)
	}
	rawCookies := strings.Split(cookieRawData, "; ")
	for _, rawCookie := range rawCookies {
		if rawCookie == "" {
			continue
		}
		if !strings.Contains(rawCookie, "=") {
			continue
		}
		items := strings.Split(rawCookie, "=")
		transferred = append(transferred, &proto.NetworkCookieParam{Name: items[0], Value: items[1], Domain: domain})
	}
	return transferred
}

func EvalOnPage(page *rod.Page, evalJs string) (*proto.RuntimeRemoteObject, error) {
	elementObj, err := proto.RuntimeEvaluate{
		IncludeCommandLineAPI: true,
		ReturnByValue:         true,
		Expression:            evalJs,
	}.Call(page)
	if err != nil {
		return nil, utils.Errorf(`page eval js code error: %v`, err)
	}
	if elementObj.Result == nil {
		return nil, utils.Error(`page eval js code result null`)
	}
	return elementObj.Result, nil
}
