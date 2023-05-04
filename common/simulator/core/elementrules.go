package core

import (
	"github.com/yaklang/yaklang/common/simulator/config"
	"strings"
)

var CheckDisplayFunctions = []func(*GeneralElement) bool{
	normalDisplayCheck,
}

var KeywordRules = map[string][]func(*GeneralElement) bool{
	"password": passwdSlice,
}

var GeneralElementRules = map[string][]func(*GeneralElement) GeneralElements{
	"button": generalButtonRules,
}

var GeneralElementRulesFromPage = map[string][]func(*GeneralPage) GeneralElements{
	"button": generalButtonRulesFromPage,
}

func normalDisplayCheck(generalElement *GeneralElement) bool {
	//elementType, _ := generalElement.GetAttribute("type")
	//if elementType == "hidden" {
	//	return false
	//}
	//style, _ := generalElement.GetAttribute("style")
	//if style == "" {
	//	return true
	//}
	//reg := regexp.MustCompile("\\s+")
	//if strings.Contains(reg.ReplaceAllString(style, ""), "display:none") {
	//	return false
	//}
	//return true
	visible, err := generalElement.element.Visible()
	if err != nil {
		return false
	}
	return visible
}

var passwdSlice = []func(generalElement *GeneralElement) bool{
	checkPasswd,
}

func checkPasswd(generalElement *GeneralElement) bool {
	elementType, _ := generalElement.GetAttribute("type")
	if elementType == "password" {
		return true
	}
	return false
}

func generalButtonElements(element *GeneralElement) GeneralElements {
	//return nil
	generalElements := make(GeneralElements, 0)
	buttons, err := element.element.Elements("button")
	if err == nil {
		for _, button := range buttons {
			buttonElement := CreateElement(button, element.page)
			if buttonElement.CheckDisplay() == false {
				continue
			}
			generalElements = append(generalElements, buttonElement)
		}
	}
	inputs, err := element.element.Elements("input")
	if err == nil {
		for _, input := range inputs {
			inputType := GetAttribute(input, "type")
			if inputType == "submit" || inputType == "button" {
				inputElement := CreateElement(input, element.page)
				if inputElement.CheckDisplay() == false {
					continue
				}
				generalElements = append(generalElements, inputElement)
			}
		}
	}
	return generalElements
}

var generalButtonRules = []func(*GeneralElement) GeneralElements{
	generalButtonElements,
}

func picInputButton(page *GeneralPage) GeneralElements {
	inputs, err := page.currentPage.Elements("input")
	if err != nil {
		return nil
	}
	generalElements := make(GeneralElements, 0)
	keywordDict := config.KeywordDict["login"]
	for _, input := range inputs {
		elementInfo := GetWholeAttributesStr(input)
		for _, keyword := range keywordDict {
			if strings.Contains(elementInfo, keyword) {
				generalElements = append(generalElements, CreateElement(input, page))
				break
			}
		}
	}
	return generalElements
}

func generalButtonElementsFromPage(page *GeneralPage) GeneralElements {
	//return nil
	generalElements := make(GeneralElements, 0)
	buttons, err := page.currentPage.Elements("button")
	if err == nil {
		for _, button := range buttons {
			buttonElement := CreateElement(button, page)
			if buttonElement.CheckDisplay() == false {
				continue
			}
			generalElements = append(generalElements, buttonElement)
		}
	}
	inputs, err := page.currentPage.Elements("input")
	if err == nil {
		for _, input := range inputs {
			inputType := GetAttribute(input, "type")
			if inputType == "submit" || inputType == "button" {
				inputElement := CreateElement(input, page)
				if inputElement.CheckDisplay() == false {
					continue
				}
				generalElements = append(generalElements, inputElement)
			}
		}
	}
	return generalElements
}

var generalButtonRulesFromPage = []func(*GeneralPage) GeneralElements{
	generalButtonElementsFromPage,
	//picInputButton,
}
