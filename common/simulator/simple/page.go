package simple

import (
	"encoding/base64"
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

type VPage struct {
	page    *rod.Page
	timeout int
}

func (page *VPage) Navigate(urlStr string, waitFor string) error {
	var err error
	page.page = page.page.Timeout(time.Duration(page.timeout) * time.Second)
	err = page.page.Navigate(urlStr)
	if err != nil {
		return utils.Errorf("page navigate %s error: %v", urlStr, err)
	}
	if waitFor != "" {
		err = page.page.WaitElementsMoreThan(waitFor, 0)
	} else {
		err = page.page.WaitLoad()
	}
	if err != nil {
		return utils.Errorf("page wait load error: %v", err)
	}
	//page.page = page.page.CancelTimeout()
	return nil
}

// Element 返回页面中匹配css selector的一个元素
//
//	Example:
//	```
//	page, _ = browser.Navigate("https://example.com", "")
//	element, _ = page.Element("#pageName") // 匹配id为pageName的元素
//	```
func (page *VPage) Element(selector string) (*VElement, error) {
	elements, err := page.page.Elements(selector)
	if err != nil {
		return nil, utils.Errorf("element find error: %s", err)
	}
	if len(elements) == 0 {
		return nil, utils.Errorf("element not found")
	}
	return &VElement{element: elements.First()}, nil
}

// Elements 返回页面中匹配css selector的所有元素
//
//	Example:
//	```
//	page, _ = browser.Navigate("https://example.com", "")
//	elements, _ = page.Element("p") // 匹配所有p标签元素
//	```
func (page *VPage) Elements(selector string) (VElements, error) {
	var result VElements
	elements, err := page.page.Elements(selector)
	if err != nil {
		return result, utils.Errorf("elements find error: %s", err)
	}
	for _, element := range elements {
		result = append(result, &VElement{element: element})
	}
	return result, nil
}

// Click 点击页面中css selector匹配到的元素
//
// 同 element.Click()
func (page *VPage) Click(selector string) error {
	element, err := page.Element(selector)
	if err != nil {
		return utils.Errorf("click element find error: %s", err)
	}
	err = element.Click()
	if err != nil {
		return utils.Errorf("click element error: %s", err)
	}
	return page.page.WaitLoad()
}

// Input 在页面中css selector匹配到的元素中输入内容
//
// 同 element.Input(string)
func (page *VPage) Input(selector, inputStr string) error {
	element, err := page.Element(selector)
	if err != nil {
		return utils.Errorf("input element find error: %s", err)
	}
	err = element.Input(inputStr)
	if err != nil {
		return utils.Errorf("input element error: %s", err)
	}
	return page.page.WaitLoad()
}

// HTML 返回整个页面的html内容
//
//	Example:
//	```
//	page, _ = browser.Navigate("https://example.com", "")
//	html, _ = page.HTML()
//	```
func (page *VPage) HTML() (string, error) {
	html, err := page.page.HTML()
	if err != nil {
		return "", utils.Errorf("get page html error: %s", err)
	}
	return html, nil
}

func (page *VPage) ScreenShot() (string, error) {
	bin, err := page.page.Screenshot(false, nil)
	if err != nil {
		return "", utils.Errorf("get page screenshot error: %s", err)
	}
	pngBase64 := base64.StdEncoding.EncodeToString(bin)
	return "data:image/png;base64," + pngBase64, nil
}

func (page *VPage) Close() error {
	return page.page.Close()
}
