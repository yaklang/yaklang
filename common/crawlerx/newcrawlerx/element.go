// Package newcrawlerx
// @Author bcy2007  2023/3/7 16:25
package newcrawlerx

import (
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"
)

func (starter *BrowserStarter) clickElementOnPageBySelector(page *rod.Page, selector string) bool {
	status, element, err := page.Has(selector)
	//element, err := page.Element(selector)
	if err != nil {
		log.Infof("on page element: %s", err)
		return false
	}
	if !status {
		log.Infof("on page element: %s not found", selector)
	}
	if element == nil {
		info := page.MustInfo()
		var url string
		if info != nil {
			url = info.URL
		}
		log.Errorf("on page %s element %s not found.", url, selector)
		return false
	}
	if visible, _ := element.Visible(); !visible {
		return false
	}
	if starter.elementCheck != nil && !starter.elementCheck(element) {
		return false
	}
	//element.Click(proto.InputMouseButtonLeft)
	element.Eval(`this.click()`)
	page.MustWaitLoad()
	time.Sleep(500 * time.Millisecond)
	return true
}

func (starter *BrowserStarter) elementCheckGenerate() func(*rod.Element) bool {
	if len(starter.baseConfig.sensitiveWords) == 0 {
		return nil
	}
	var propertyList = []string{"innerHTML", "value"}
	return func(element *rod.Element) bool {
		var resultStr string
		for _, property := range propertyList {
			subStr, _ := getProperty(element, property)
			if subStr != "" {
				resultStr += ";" + subStr
			}
		}
		result, word := StringArrayCover(starter.baseConfig.sensitiveWords, resultStr)
		if result {
			var url string
			page := element.Page()
			if page != nil {
				info := page.MustInfo()
				if info != nil {
					url = info.URL
				}
			}
			log.Infof(`In url %s element %s do not click because of sensitive word: %s`, url, element.Object.Description, word)
			return false
		}
		return true
	}
}

func getInputSubmitElementSelectors(page *rod.Page) []string {
	searchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"submit",
			},
		},
	}
	elements, _ := customizedGetElement(page, searchInfo)
	return getElementsSelectors(elements)
}

// get clickable element from event listener
// include button && input type=button
// but not contains input type=submit
func getClickableElementSelectors(page *rod.Page) []string {
	return nil
}

func GetDefaultClickElementSelectors_(page *rod.Page) ([]string, error) {
	searchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"submit",
				"button",
			},
		},
		"button": {},
	}
	elements, err := customizedGetElement(page, searchInfo)
	if err != nil {
		return []string{}, utils.Errorf("customized get element error: %s", err)
	}
	return getElementsSelectors(elements), nil
}

func GetDefaultClickElementSelectors(page *rod.Page) []string {
	searchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"submit",
				"button",
			},
		},
		"button": {},
	}
	elements, _ := customizedGetElement(page, searchInfo)
	return getElementsSelectors(elements)
}

// map[{{ tagName }}]map[{{ element attribute }}]{{ attribute results }}
func customizedGetElement(page *rod.Page, searchInfo map[string]map[string][]string) (rod.Elements, error) {
	resultElements := make([]*rod.Element, 0)
	for tagName, tagInfo := range searchInfo {
		elements, err := customizedCheckTagElements(page, tagName, tagInfo)
		if err != nil {
			continue
		}
		resultElements = append(resultElements, elements...)
	}
	return resultElements, nil
}

func customizedCheckTagElements(page *rod.Page, tagName string, tagInfo map[string][]string) (rod.Elements, error) {
	elements, err := page.Elements(tagName)
	if err != nil {
		return nil, utils.Errorf("page %s get tag % element error: %s", page, tagName, err)
	}
	if len(tagInfo) == 0 {
		return elements, nil
	}
	resultElements := make([]*rod.Element, 0)
	for _, element := range elements {
		if customizedCheckElementAttribute(element, tagInfo) {
			resultElements = append(resultElements, element)
		}
	}
	return resultElements, nil
}

func customizedCheckElementAttribute(element *rod.Element, attributeInfo map[string][]string) bool {
	for attribute, attributeList := range attributeInfo {
		info, _ := getAttribute(element, attribute)
		if info == "" {
			continue
		}
		info = strings.ToLower(info)
		if StringArrayContains(attributeList, info) {
			return true
		}
	}
	return false
}

func testUploadFile(element *rod.Element, filePath string) error {
	return element.SetFiles([]string{filePath})
}
