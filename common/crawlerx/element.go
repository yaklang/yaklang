// Package crawlerx
// @Author bcy2007  2023/7/13 11:32
package crawlerx

import (
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
	"time"
)

func (starter *BrowserStarter) clickElementOnPageBySelector(page *rod.Page, selector string) bool {
	info, err := page.Info()
	if err != nil {
		log.Errorf("page %v get info error: %v", page, err)
		return false
	}
	var url, title string
	if info != nil {
		url = info.URL
		title = info.Title
	}
	if strings.HasSuffix(url, "#") {
		url = url[:len(url)-1]
	}
	status, element, err := page.Has(selector)
	//element, err := page.Element(selector)
	if err != nil {
		log.Debugf("On page %s element: %s", url, err)
		return false
	}
	if !status {
		log.Debugf("On page %s element: %s not found", url, selector)
		return false
	}
	if element == nil {
		log.Debugf("On page %s element %s not found.", url, selector)
		return false
	}
	if starter.aiInputUrl != "" {
		var elementHTML string
		elementHTML, err = element.HTML()
		if err != nil {
			log.Debugf("On page %s element %s get html error: %s", url, selector, err)
			return false
		}
		if len(elementHTML) > 200 {
			reg, _ := regexp.Compile("style=\".+?\"|size=\".+?\"")
			elementHTML = reg.ReplaceAllLiteralString(elementHTML, "")[:200]
		}
		parent, _ := element.Parent()
		if parent != nil {
			//text += parent.
			class, _ := getAttribute(parent, "class")
			elementHTML += " " + class
			grandParent, _ := element.Parent()
			if grandParent != nil {
				grandClass, _ := getAttribute(grandParent, "class")
				elementHTML += " " + grandClass
			}
		}
		var output AIInputResult
		var aiInput string
		aiInput = title + " " + elementHTML
		output, _ = starter.getElementInputByAI(aiInput)
		if output.DButt == true {
			log.Debugf("On page %s element %s is dangerous button", url, selector)
			return false
		}
	}
	if visible, _ := element.Visible(); !visible {
		log.Debugf(`On page %s element %s not visible`, url, selector)
		return false
	}
	if starter.elementCheck != nil && !starter.elementCheck(element) {
		log.Debugf(`On page %s element check failed`, url)
		return false
	}
	//element.Click(proto.InputMouseButtonLeft)
	_, err = element.Eval(`()=>this.click()`)
	if err != nil {
		log.Errorf(`click element %v error: %v`, element, err.Error())
		return false
	}
	time.Sleep(500 * time.Millisecond)
	//page.MustWaitLoad()
	err = page.WaitLoad()
	if err != nil {
		log.Errorf("page %v wait load error: %v", page, err)
		return false
	}
	return true
}

func (starter *BrowserStarter) elementCheckGenerate() func(*rod.Element) bool {
	if len(starter.baseConfig.sensitiveWords) == 0 {
		return nil
	}
	var propertyList = []string{"innerHTML", "value", "name"}
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
				//info := page.MustInfo()
				info, err := page.Info()
				if err != nil {
					log.Errorf("get page %v info error: %v", url, err)
					return false
				}
				if info != nil {
					url = info.URL
				}
				if strings.HasSuffix(url, "#") {
					url = url[:len(url)-1]
				}
			}
			log.Debugf(`In url %s element %s do not click because of sensitive word: %s`, url, element.Object.Description, word)
			return false
		}
		return true
	}
}

// map[{{ tagName }}]map[{{ element attribute }}]{{ attribute results }}
func customizedGetElement(page *rod.Page, searchInfo []ElementInfo) (rod.Elements, error) {
	resultElements := make([]*rod.Element, 0)
	for _, searchItem := range searchInfo {
		elements, err := customizedCheckTagElements(page, searchItem)
		if err != nil {
			continue
		}
		resultElements = append(resultElements, elements...)
	}
	return resultElements, nil
}

func customizedCheckTagElements(page *rod.Page, elementInfo ElementInfo) (rod.Elements, error) {
	elements, err := page.Elements(elementInfo.Tag)
	if err != nil {
		return nil, utils.Errorf("page %s get tag %s element error: %s", page, elementInfo.Tag, err)
	}
	if len(elementInfo.Attributes) == 0 {
		return elements, nil
	}
	resultElements := make([]*rod.Element, 0)
	for _, element := range elements {
		if customizedCheckElementAttribute(element, elementInfo.Attributes) {
			resultElements = append(resultElements, element)
		}
	}
	return resultElements, nil
}

func customizedCheckElementAttribute(element *rod.Element, attributeInfo []ElementAttribute) bool {
	for _, attributeItem := range attributeInfo {
		info, _ := getAttribute(element, attributeItem.Name)
		if info == "" {
			continue
		}
		info = strings.ToLower(info)
		if StringArrayContains(attributeItem.Info, info) {
			return true
		}
	}
	return false
}
