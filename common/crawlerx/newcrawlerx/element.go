// Package newcrawlerx
// @Author bcy2007  2023/3/7 16:25
package newcrawlerx

import (
	"github.com/go-rod/rod"
	"strings"
	"github.com/yaklang/yaklang/common/utils"
)

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

func GetDefaultClickElementSelectors(page *rod.Page) []string {
	searchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"submit",
				//"button",
			},
		},
		//"button": {},
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
