package core

import (
	"github.com/go-rod/rod"
	"time"
)

func (crawler *CrawlerX) InputPage(page *GeneralPage) error {
	status, _, err := page.Has("input")
	if err != nil {
		return err
	}
	if !status {
		return nil
	}
	inputs, err := page.Elements("input")
	if err != nil {
		return err
	}
	for _, input := range inputs {
		if !crawler.NewVisible(input) {
			continue
		}
		crawler.DoInput(input)
	}
	return nil
}

func (crawler *CrawlerX) ClickPage(page *GeneralPage) error {
	originUrl := page.GetCurrentUrl()
	buttonSelectors := crawler.GetButtonsSelectors(page)
	for _, buttonSelector := range buttonSelectors {
		crawler.ClickButton(page, buttonSelector)
		currentUrl := page.GetCurrentUrl()
		if originUrl != currentUrl {
			page.GoDeeper()
			if page.CurrentDepth() <= crawler.maxDepth {
				crawler.VisitPage(page)
			}
			page.GoBack()
			if page.GetCurrentUrl() != originUrl {
				page.GoBack()
			}
		}
	}
	//crawler.ExtractUrl(page)
	return nil
}

func (crawler *CrawlerX) GetButtonsSelectors(page *GeneralPage) []string {
	buttons, _ := page.Elements("button")
	buttonSelectors := crawler.GetElementsSelectors(buttons)
	inputs, _ := page.Elements("input")
	for _, input := range inputs {
		inputType, _ := input.Attribute("type")
		if inputType != nil && (*inputType == "submit" || *inputType == "button") {
			buttonSelectors = append(buttonSelectors, crawler.GetElementSelector(input))
		}
	}
	return buttonSelectors
}

func (crawler *CrawlerX) ClickButton(page *GeneralPage, buttonSelector string) {
	// tbc
	// button selector detect
	buttonElements, _ := page.Elements(buttonSelector)
	if buttonElements == nil {
		return
	}
	if len(buttonElements) == 0 {
		return
	}
	buttonElement := buttonElements[0]
	//buttonElement, _ := page.Element(buttonSelector)
	if !crawler.NewVisible(buttonElement) {
		return
	}
	wait := page.WaitRequestIdle(time.Second, nil, nil)
	buttonElement.Eval("()=>this.click()")
	//buttonElement.Click(proto.InputMouseButtonLeft)
	wait()
}

func clickButton(page *rod.Page, buttonSelector string) {
	buttonElements, _ := page.Elements(buttonSelector)
	if buttonElements == nil {
		return
	}
	if len(buttonElements) == 0 {
		return
	}
	buttonElement := buttonElements[0]
	if visible, _ := buttonElement.Visible(); !visible {
		return
	}
	wait := page.WaitRequestIdle(time.Second, nil, nil)
	buttonElement.Eval("()=>this.click()")
	wait()
}

func getButtonSelectors(page *rod.Page) []string {
	buttons, _ := page.Elements("button")
	buttonSelectors := getElementsSelectors(buttons)
	inputs, _ := page.Elements("input")
	for _, input := range inputs {
		inputType, _ := input.Attribute("type")
		if inputType != nil && (*inputType == "submit" || *inputType == "button") {
			buttonSelectors = append(buttonSelectors, getElementSelector(input))
		}
	}
	return buttonSelectors
}

func getElementSelector(element *rod.Element) string {
	selector, err := element.Eval(getSelector)
	if err != nil {
		return ""
	}
	return selector.Value.Str()
}

func getElementsSelectors(elements rod.Elements) []string {
	selectors := make([]string, 0)
	for _, element := range elements {
		selector := getElementSelector(element)
		if selector != "" {
			selectors = append(selectors, selector)
		}
	}
	return selectors
}
