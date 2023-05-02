package core

import (
	"encoding/base64"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

func (generalPage *GeneralPage) GeneralFindElements(keyword string) (*GeneralElements, error) {
	rules, ok := GeneralElementRulesFromPage[keyword]
	if ok {
		targetElements := make(GeneralElements, 0)
		for _, rule := range rules {
			tempElements := rule(generalPage)
			targetElements = append(targetElements, tempElements...)
		}
		return &targetElements, nil
	}
	targetElements, err := generalPage.FindElements(keyword)
	if err != nil {
		return nil, utils.Errorf("page %s get general element %s error %s", generalPage.url, keyword, err)
	}
	return targetElements, nil
}

func (generalPage *GeneralPage) GeneralFindElement(keyword string) (*GeneralElement, error) {
	elements, err := generalPage.GeneralFindElements(keyword)
	if err != nil {
		return nil, utils.Errorf("page %s get general element %s error %s", generalPage.url, keyword, err)
	}
	if elements.Empty() {
		return nil, utils.Errorf("page %s get general element %s not found", generalPage.url, keyword)
	}
	return elements.First(), nil
}

func (generalPage *GeneralPage) FindElements(keyword string) (*GeneralElements, error) {
	ges := make(GeneralElements, 0)
	elements, err := generalPage.currentPage.Elements(keyword)
	if err != nil {
		return nil, utils.Errorf("page %s get elements %s error: %s", generalPage.url, keyword, err)
	}
	for _, _element := range elements {
		readyElement := CreateElement(_element, generalPage)
		if readyElement.CheckDisplay() == false {
			continue
		}
		ges = append(ges, readyElement)
	}
	return &ges, nil
}

func (generalPage *GeneralPage) FindElement(keyword string) (*GeneralElement, error) {
	elements, err := generalPage.FindElements(keyword)
	if err != nil {
		return nil, utils.Errorf("page %s get element %s error: %s", generalPage.url, keyword, err)
	}
	if !elements.Empty() {
		return elements.First(), nil
	}
	return nil, utils.Errorf("page %s get element %s not found", generalPage.url, keyword)
}

func (generalPage *GeneralPage) StartListen() error {
	generalPage.createWait()
	_, err := generalPage.currentPage.Eval(OBSERVER)
	if err != nil {
		return utils.Errorf("eval start listen error: %s", err)
	}
	return nil
}

func (generalPage *GeneralPage) StopListen() (string, error) {
	generalPage.Wait()
	result, err := generalPage.currentPage.Eval(OBSERVERRESULT)
	if err != nil {
		return "", utils.Errorf("get observer result error: %s", err)
	}
	return result.Value.String(), nil
}

func (generalPage *GeneralPage) StopListenWithBytes() ([]byte, error) {
	//generalPage.Wait()
	//log.Info("stop!")
	result, err := generalPage.currentPage.Eval(OBSERVERRESULT)
	//log.Info(result)
	if err != nil {
		return nil, utils.Errorf("get observer result error: %s", err)
	}
	//return result.Value.String(), nil
	resultStr := result.Value.Str()
	return []byte(resultStr), nil
}

func (generalPage *GeneralPage) Wait() {
	generalPage.wait()
}

func (generalPage *GeneralPage) GetLoginButton() (*GeneralElement, error) {
	// find button
	buttonElements, err := generalPage.FindElements("button")
	if err != nil {
		return nil, utils.Errorf("page %s get button element error: %s", generalPage, err)
	}
	if buttonElements.Single() {
		return buttonElements.First(), nil
	} else if !buttonElements.Empty() {
		return buttonElements.First(), nil
	}

	//find input
	inputElements, err := generalPage.FindElements("input")
	if err != nil {
		return nil, utils.Errorf("page %s get input element error: %s", generalPage, err)
	}
	for _, inputElement := range *inputElements {
		inputType, _ := inputElement.GetAttribute("type")
		if inputType == "submit" || inputType == "button" {
			return inputElement, nil
		}
	}
	return nil, utils.Errorf("page %s cannot find related login button", generalPage)
}

func (generalPage *GeneralPage) Click(elementStr string) {
	element, err := generalPage.FindElement(elementStr)
	if err != nil {
		log.Errorf("click find element error: %s", err)
		return
	}
	if element == nil {
		log.Errorf("element %s not found. click stop", elementStr)
		return
	}
	element.Click()
}

func (generalPage *GeneralPage) Input(elementStr, inputStr string) {
	element, err := generalPage.FindElement(elementStr)
	if err != nil {
		log.Errorf("input find element error: %s", err)
		return
	}
	if element == nil {
		log.Errorf("element %s not found. input stop", elementStr)
		return
	}
	element.Input(inputStr)
}

func (generalPage *GeneralPage) Screenshot(filePath string) {
	generalPage.currentPage.MustScreenshot(filePath)
}

func (generalPage *GeneralPage) ScreenShotResult() (string, error) {
	pngBytes, err := generalPage.currentPage.Screenshot(false, nil)
	if err != nil {
		return "", utils.Errorf("page %s screen shot error: %s", generalPage.url, err)
	}
	pngBase64 := base64.StdEncoding.EncodeToString(pngBytes)
	return "data:image/png;base64," + pngBase64, nil
}

func (generalPage *GeneralPage) Test() {
	//return generalPage.currentPage
	generalPage.Click("")
}

func (generalPage *GeneralPage) CurrentURL() string {
	return generalPage.currentPage.MustEval("()=>document.URL").Str()
}

func (generalPage *GeneralPage) HTML() string {
	//return generalPage.currentPage.HTML()
	html, err := generalPage.currentPage.HTML()
	if err != nil {
		log.Errorf("get page %s html error: %s", generalPage.url, err)
		return ""
	}
	return html
}
