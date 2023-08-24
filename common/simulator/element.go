// Package simulator
// @Author bcy2007  2023/8/17 16:19
package simulator

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

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
		return nil, err
	}
	if len(tagInfo) == 0 {
		return elements, nil
	}
	resultElements := make(rod.Elements, 0)
	for _, element := range elements {
		if customizedCheckElementAttribute(element, tagInfo) {
			resultElements = append(resultElements, element)
		}
	}
	return resultElements, nil
}

func customizedCheckElementAttribute(element *rod.Element, attributeInfo map[string][]string) bool {
	for attribute, attributeList := range attributeInfo {
		info, _ := GetAttribute(element, attribute)
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

func GetElementParam(element *rod.Element, param string) (string, error) {
	if StringArrayContains(ElementAttribute, param) {
		return GetAttribute(element, param)
	} else if StringArrayContains(ElementProperty, param) {
		return GetProperty(element, param)
	}
	return "", nil
}

func GetAttribute(element *rod.Element, attr string) (string, error) {
	attribute, err := element.Attribute(attr)
	if err != nil {
		return "", err
	}
	if attribute == nil {
		return "", nil
	}
	return *attribute, nil
}

func GetProperty(element *rod.Element, prop string) (string, error) {
	property, err := element.Property(prop)
	if err != nil {
		return "", err
	}
	if property.Nil() {
		return "", nil
	}
	return property.String(), nil
}

var getSelector = `()=>{
    let e = this;
    let domPath = Array();
    if (e.getAttribute("id")) {
        domPath.unshift('#'+e.id);
    } else {
        while (e.nodeName.toLowerCase() !== "html") {
            if(e.getAttribute("id")){
                domPath.unshift('#'+e.getAttribute("id"));
                break;
            }else if(e.tagName.toLocaleLowerCase() == "body") {
                domPath.unshift(e.tagName.toLocaleLowerCase());
            }else{
                for (i = 0; i < e.parentNode.childElementCount; i++) {
                    if (e.parentNode.children[i] == e) {
                        domPath.unshift(e.tagName.toLocaleLowerCase() + ':nth-child(' + (i + 1) + ')');
                    }
                }
            }
            e = e.parentNode;
        }
    }
	domPath = domPath.toString().replaceAll(',', '>');
    return domPath
}`

func ElementsToSelectors(elements ...*rod.Element) []string {
	selectors := make([]string, 0)
	for _, element := range elements {
		selector, err := GetSelector(element)
		if err != nil {
			log.Error(err)
			continue
		}
		selectors = append(selectors, selector)
	}
	return selectors
}

var getName = `()=>{
    let result = this.tagName.toLowerCase();
	if (this.id !== ""){
		result += "#" + this.id;
	}
	if (this.className !== ""){
		result += "." + this.className;
    }
	return result
}`

func ElementsToIds(elements ...*rod.Element) []string {
	ids := make([]string, 0)
	for _, element := range elements {
		obj, err := element.Eval(getName)
		if err != nil {
			log.Error(err)
			continue
		}
		ids = append(ids, obj.Value.String())
	}
	return ids
}

func ElementsToValue(elements rod.Elements, eval string) []string {
	results := make([]string, 0)
	for _, element := range elements {
		value, err := ElementToValue(element, eval)
		if err != nil {
			log.Error(err)
			continue
		}
		results = append(results, value)
	}
	return results
}

func ElementToValue(element *rod.Element, eval string) (string, error) {
	obj, err := element.Eval(eval)
	if err != nil {
		return "", utils.Error(err)
	}
	return obj.Value.String(), nil
}

func GetElement(page *rod.Page, selector string) (*rod.Element, error) {
	elements, err := page.Elements(selector)
	if err != nil {
		return nil, utils.Error(err)
	}
	if len(elements) == 0 {
		return nil, utils.Errorf(`%v not found`, selector)
	}
	element := elements.First()
	return element, nil
}

func ElementInput(page *rod.Page, selector, inputInfo string) (err error) {
	element, err := GetElement(page, selector)
	if err != nil {
		return
	}
	err = element.SelectAllText()
	if err != nil {
		return
	}
	err = element.Type(input.Backspace)
	if err != nil {
		return
	}
	inputKeys := []input.Key(inputInfo)
	err = element.Type(inputKeys...)
	if err != nil {
		return
	}
	return
}

func ElementClick(page *rod.Page, selector string) (err error) {
	element, err := GetElement(page, selector)
	if err != nil {
		return
	}
	return element.Click(proto.InputMouseButtonLeft)
}

func FindLatestElement(page *rod.Page, origin, tagName string, maxLevel int) (rod.Elements, error) {
	originElements, err := page.Elements(origin)
	if err != nil {
		return nil, utils.Error(err)
	}
	if len(originElements) == 0 {
		return nil, utils.Errorf(`element %v not found`, origin)
	}
	originElement := originElements.First()
	elements, err := originElement.Elements(tagName)
	if err != nil {
		return nil, utils.Error(err)
	}
	if len(elements) != 0 {
		return elements, nil
	}
	parent, err := originElement.Parent()
	if err != nil {
		return nil, utils.Error(err)
	}
	currentLevel := 0
	for parent != nil {
		elements, err := parent.Elements(tagName)
		if err != nil {
			return nil, utils.Error(err)
		}
		if len(elements) != 0 {
			return elements, nil
		}
		if currentLevel >= maxLevel {
			break
		}
		currentLevel++
		parent, err = parent.Parent()
		if err != nil {
			return nil, utils.Error(err)
		}
	}
	return nil, utils.Errorf(`cannot find %v's latest %v element`, origin, tagName)
}

func GetSelector(element *rod.Element) (string, error) {
	obj, err := element.Eval(getSelector)
	if err != nil {
		return "", utils.Error(err)
	}
	return obj.Value.String(), nil
}
