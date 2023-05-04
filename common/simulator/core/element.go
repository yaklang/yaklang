package core

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/simulator/config"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

const maxScanLevel = 3

var inputTypes = []string{
	"text", "password", "file", "hidden",
	"button", "checked", "checkbox", "radio",
	"image", "submit", "rest",
	"month", "number", "tel", "time", "week",
}

func (generalElements GeneralElements) FilteredKeywordElements(filter string) *GeneralElements {
	targetElements := make(GeneralElements, 0)
	for _, generalElement := range generalElements {
		ifRelated, err := generalElement.HasTypeKeyword(filter)
		if err != nil {
			log.Errorf("judge element whether is %s error: %s", filter, err)
			continue
		}
		if ifRelated {
			targetElements = append(targetElements, generalElement)
		}
	}
	// search from parents
	if targetElements.Length() == 0 {
		final := generalElements.filterFromParents(filter)
		if final != nil {
			targetElements = append(targetElements, final)
		}
	}
	return &targetElements
}

func (generalElements GeneralElements) filterFromParents(filter string) *GeneralElement {
	var maxLevel int = 999
	var finalElement *GeneralElement
	var filterDict = config.KeywordDict[filter]
	if len(filterDict) == 0 {
		return nil
	}
	for _, generalElement := range generalElements {
		parent, _ := generalElement.GetParent()
		var currentLevel = 0
		for parent != nil {
			status, _ := parent.HasTypeKeyword(filter)
			if status && currentLevel < maxLevel {
				//log.Info(parent)
				maxLevel = currentLevel
				finalElement = generalElement
				break
			}
			html := parent.HTML()
			if ContainsGroup(html, filterDict) && currentLevel < maxLevel {
				//log.Info(parent, html, filterDict)
				maxLevel = currentLevel
				finalElement = generalElement
				break
			}
			if currentLevel >= maxScanLevel {
				break
			}
			currentLevel++
			parent, _ = parent.GetParent()
		}
	}
	return finalElement
}

func (generalElements GeneralElements) FilteredKeywordElement(filter string) *GeneralElement {
	//return &GeneralElement{}
	var maxValue float32 = 0.0
	var relatedElement *GeneralElement = nil
	for _, generalElement := range generalElements {
		value, err := generalElement.CalculateRelevance(filter)
		if err != nil {
			log.Errorf("element %s get relevance error: %s", generalElement, err)
			continue
		}
		if value > maxValue {
			relatedElement = generalElement
			maxValue = value
		}
	}
	return relatedElement
}

func (generalElements GeneralElements) FilteredTypeElement(typeStr ...string) *GeneralElements {
	targetElements := make(GeneralElements, 0)
	for _, generalElement := range generalElements {
		attrStr, _ := generalElement.GetAttribute("type")
		attrStr = strings.ToLower(attrStr)
		if !utils.StringSliceContain(inputTypes, attrStr) {
			targetElements = append(targetElements, generalElement)
			break
		}
		if utils.StringSliceContain(typeStr, attrStr) {
			targetElements = append(targetElements, generalElement)
		}
	}
	return &targetElements
}

func (generalElements GeneralElements) ExcludeTypeElement(typeStr ...string) *GeneralElements {
	targetElements := make(GeneralElements, 0)
	for _, generalElement := range generalElements {
		attrStr, _ := generalElement.GetAttribute("type")
		attrStr = strings.ToLower(attrStr)
		if !utils.StringSliceContain(typeStr, attrStr) {
			targetElements = append(targetElements, generalElement)
		}
	}
	return &targetElements
}

func (generalElement *GeneralElement) GetParent() (*GeneralElement, error) {
	parent, err := generalElement.element.Parent()
	if err != nil {
		return nil, utils.Errorf("element: %s find parent error: %s", generalElement, err)
	}
	if parent == nil {
		return nil, nil
	}
	parentElement := CreateElement(parent, generalElement.page)
	return parentElement, nil
}

func (generalElement *GeneralElement) GetElement(elementStr string) (*GeneralElement, error) {
	generalElements, err := generalElement.GetElements(elementStr)
	if err != nil {
		return nil, utils.Errorf("element %s get element %s error: %s", generalElement, elementStr, err)
	}
	if !generalElements.Empty() {
		return generalElements.First(), nil
	}
	return nil, nil
}
func (generalElement *GeneralElement) GetElements(elementStr string) (*GeneralElements, error) {
	tempElements, err := generalElement.element.Elements(elementStr)
	if err != nil {
		return nil, utils.Errorf("element %s get elements %s error: %s", generalElement, elementStr, err)
	}
	generalElements := generalElement.elementsToGeneralElements(tempElements)
	return generalElements, nil
}

func (generalElement *GeneralElement) GeneralGetElement(elementStr string) (*GeneralElement, error) {
	generalElements, err := generalElement.GeneralGetElements(elementStr)
	if err != nil {
		return nil, utils.Errorf("element %s get general element %s error: %s", generalElement, elementStr, err)
	}
	if !generalElements.Empty() {
		return generalElements.Last(), nil
	}
	return nil, nil
}

func (generalElement *GeneralElement) GeneralGetElements(elementStr string) (*GeneralElements, error) {
	rules, ok := GeneralElementRules[elementStr]
	if ok {
		targetElements := make(GeneralElements, 0)
		for _, rule := range rules {
			tempElements := rule(generalElement)
			targetElements = append(targetElements, tempElements...)
		}
		return &targetElements, nil
	}
	targetElement, err := generalElement.GetElements(elementStr)
	if err != nil {
		return nil, utils.Errorf("element %s get general elements %s error: %s", generalElement, elementStr, err)
	}
	return targetElement, nil
}

func (generalElement *GeneralElement) HasTypeKeyword(typeStr string) (bool, error) {
	// special rules
	// checking before use some keywords to judge
	spFunctions, ok := KeywordRules[typeStr]
	if ok {
		for _, function := range spFunctions {
			if function(generalElement) {
				return true, nil
			}
		}
	}
	// end sp
	typeKeywords, ok := config.SimpleKeywordDict[typeStr]
	if !ok {
		return false, utils.Errorf("get error type str: %s", typeStr)
	}
	tempStr := generalElement.GetWholeAttributesStr()
	//log.Info(tempStr)
	if tempStr == "" {
		return false, nil
	}
	for _, typeKeyword := range typeKeywords {
		if strings.Contains(tempStr, typeKeyword) {
			//log.Info(tempStr, typeKeyword)
			return true, nil
		}
	}
	return false, nil
}

func (generalElement *GeneralElement) GetWholeAttributesStr() string {
	attributes := config.ElementAttribute
	var attributesStr string
	for _, attribute := range attributes {
		tempStr, err := generalElement.GetAttribute(attribute)
		if err == nil {
			attributesStr += tempStr + ";"
		}
	}
	return attributesStr
}

func (generalElement *GeneralElement) CalculateRelevance(typeStr string) (float32, error) {
	typeKeywords, ok := config.KeywordDict[typeStr]
	if !ok {
		return 0.0, utils.Errorf("get error type str: %s", typeStr)
	}
	attributes := config.KeywordAttribute
	var likeValue float32
	for _, attribute := range attributes {
		attributeStr, err := generalElement.GetValue(attribute)
		if err != nil || attributeStr == "" {
			continue
		}
		tempValue := calculateGroupRelevance(attributeStr, typeKeywords)
		if tempValue > likeValue {
			likeValue = tempValue
		}
	}
	return likeValue, nil
}

func (generalElement *GeneralElement) GetValue(value string) (string, error) {
	if utils.StringArrayContains(config.ElementAttribute, value) {
		return generalElement.GetAttribute(value)
	} else if utils.StringArrayContains(config.ElementProperty, value) {
		return generalElement.GetProperty(value)
	}
	return "", utils.Errorf("value %s not valid", value)
}

func (generalElement *GeneralElement) GetAttribute(attr string) (string, error) {
	attribute, err := generalElement.element.Attribute(attr)
	if err != nil {
		return "", utils.Errorf("get element attribute error: %s", err)
	}
	if attribute == nil {
		return "", nil
	}
	result := strings.ToLower(*attribute)
	return result, nil
}

func (generalElement *GeneralElement) GetAttributeOrigin(attr string) (string, error) {
	attribute, err := generalElement.element.Attribute(attr)
	if err != nil {
		return "", utils.Errorf("get element attribute error: %s", err)
	}
	if attribute == nil {
		return "", nil
	}
	return *attribute, nil
}

func (generalElement *GeneralElement) GetProperty(propertyStr string) (string, error) {
	//generalElement.element.Attribute(
	property, err := generalElement.element.Property(propertyStr)
	if err != nil {
		return "", utils.Errorf("get element %s property error: %s", generalElement, propertyStr)
	}
	if property.Nil() {
		return "", nil
	}

	return strings.ToLower(property.Str()), nil
}

func (generalElement *GeneralElement) CheckDisplay() bool {
	for _, checkFunction := range CheckDisplayFunctions {
		if checkFunction(generalElement) {
			return true
		}
	}
	return false
}

func (generalElement *GeneralElement) GetLatestElement(elementStr string, maxLevel int) (*GeneralElement, error) {
	tempElement, err := generalElement.GetElement(elementStr)
	if err != nil {
		return nil, utils.Errorf("element %s get latest element %s error: %s", generalElement, elementStr, err)
	}
	if tempElement != nil {
		return tempElement, nil
	}
	var currentLevel int
	parent, _ := generalElement.GetParent()
	for parent != nil {
		tempElement, err = parent.GetElement(elementStr)
		if err != nil {
			return nil, utils.Errorf("element parent %s get latest element %s error: %s", parent, elementStr, err)
		}
		if tempElement != nil {
			return tempElement, nil
		}
		currentLevel++
		if currentLevel >= maxLevel {
			break
		}
		parent, _ = parent.GetParent()
	}
	return nil, nil
}

func (generalElement *GeneralElement) GeneralGetLatestElement(elementStr string, maxLevel int) (*GeneralElement, error) {
	tempElement, err := generalElement.GeneralGetElement(elementStr)
	if err != nil {
		return nil, utils.Errorf("element %s get latest general element %s error: %s", generalElement, elementStr, err)
	}
	if tempElement != nil {
		return tempElement, nil
	}
	var currentLevel int
	parent, _ := generalElement.GetParent()
	for parent != nil {
		tempElement, err = parent.GeneralGetElement(elementStr)
		if err != nil {
			return nil, utils.Errorf("element parent %s get latest general element %s error: %s", parent, elementStr, err)
		}
		if tempElement != nil {
			return tempElement, nil
		}
		currentLevel++
		if currentLevel >= maxLevel {
			break
		}
		parent, _ = parent.GetParent()
	}
	return nil, nil
}

func (generalElement *GeneralElement) GeneralGetLatestElements(elementStr string, maxLevel int) (*GeneralElements, error) {
	tempElements, err := generalElement.GeneralGetElements(elementStr)
	if err != nil {
		return tempElements, utils.Errorf("element %s get latest general elements %s error: %s", generalElement, elementStr, err)
	}
	if tempElements.Length() != 0 {
		return tempElements, nil
	}
	var currentLevel int
	parent, _ := generalElement.GetParent()
	for parent != nil {
		tempElements, err = parent.GeneralGetElements(elementStr)
		if err != nil {
			return tempElements, utils.Errorf("element parent %s get latest general elements %s error: %s", parent, elementStr, err)
		}
		if tempElements.Length() != 0 {
			return tempElements, nil
		}
		currentLevel++
		if currentLevel >= maxLevel {
			break
		}
		parent, _ = parent.GetParent()
	}
	return tempElements, nil
}

func (generalElement *GeneralElement) Click() error {
	//log.Info(generalElement.element)
	err := generalElement.element.Click(proto.InputMouseButtonLeft)
	//_, err := generalElement.element.Eval("()=>this.click()")
	if err != nil {
		err = generalElement.Redirect()
		if err != nil {
			return err
		}
		//_, err = generalElement.element.Eval("()=>this.click()")
		err = generalElement.element.Click(proto.InputMouseButtonLeft)
		if err != nil {
			return err
		}
	}
	return nil
}

func (generalElement *GeneralElement) Input(inputStr string) {
	//generalElement.element.MustSelectAllText().MustType(input.Backspace)
	err := generalElement.element.SelectAllText()
	if err != nil {
		generalElement.Redirect()
	} else {
		generalElement.element.Type(input.Backspace)
	}
	inputKeys := []input.Key(inputStr)
	generalElement.element.Type(inputKeys...)
}

func (generalElement *GeneralElement) elementsToGeneralElements(elements rod.Elements) *GeneralElements {
	generalElements := make(GeneralElements, 0)
	for _, element := range elements {
		tempGeneralElement := CreateElement(element, generalElement.page)
		if tempGeneralElement.CheckDisplay() == false {
			continue
		}
		generalElements = append(generalElements, tempGeneralElement)
	}
	return &generalElements
}

func (generalElement *GeneralElement) Eval(js string) string {
	result, _ := generalElement.element.Eval(js)
	return result.Value.Str()
}

func (generalElement *GeneralElement) Redirect() error {
	selector := generalElement.selector
	if selector == "" {
		return utils.Errorf("selector not found, redirect failed")
	}
	elements, err := generalElement.page.currentPage.Elements(selector)
	if err != nil {
		return utils.Errorf("cannot find element from selector: %s", selector)
	}
	if len(elements) == 0 {
		return utils.Errorf("no selector %s elements found.", selector)
	}
	element := elements[0]
	generalElement.element = element
	return nil
}
