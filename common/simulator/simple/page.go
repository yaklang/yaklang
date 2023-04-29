package simple

import (
	"encoding/base64"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"yaklang/common/utils"
)

type VPage struct {
	page *rod.Page
}

func (page *VPage) Navigate(urlStr string) {
	page.page.Navigate(urlStr)
	page.page.WaitLoad()
}

func (page *VPage) Element(selector string) (*rod.Element, error) {
	element, err := page.page.Element(selector)
	if err != nil {
		return nil, utils.Errorf("element find error: %s", err)
	}
	return element, nil
}

func (page *VPage) Click(selector string) error {
	element, err := page.Element(selector)
	if err != nil {
		return utils.Errorf("click element find error: %s", err)
	}
	err = element.Click(proto.InputMouseButtonLeft)
	if err != nil {
		return utils.Errorf("element click error: %s", err)
	}
	page.page.WaitLoad()
	return nil
}

func (page *VPage) Input(selector, inputStr string) error {
	element, err := page.Element(selector)
	if err != nil {
		return utils.Errorf("input element find error: %s", err)
	}
	err = element.Input(inputStr)
	if err != nil {
		return utils.Errorf("element input error: %s", err)
	}
	return nil
}

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
