package simple

import (
	"encoding/base64"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
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
		log.Error(err)
		return err
	}
	if waitFor != "" {
		err = page.page.WaitElementsMoreThan(waitFor, 0)
	} else {
		err = page.page.WaitLoad()
	}
	if err != nil {
		log.Error(err)
		return err
	}
	page.page = page.page.CancelTimeout()
	return nil
}

func (page *VPage) Element(selector string) (*VElement, error) {
	element, err := page.page.Element(selector)
	if err != nil {
		return nil, utils.Errorf("element find error: %s", err)
	}
	return &VElement{element: element}, nil
}

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

func (page *VPage) Click(selector string) error {
	element, err := page.Element(selector)
	if err != nil {
		return utils.Errorf("click element find error: %s", err)
	}
	err = element.element.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return utils.Errorf("element click error: %s", err)
	}
	err = page.page.WaitLoad()
	return err
}

func (page *VPage) Input(selector, inputStr string) error {
	element, err := page.Element(selector)
	if err != nil {
		return utils.Errorf("input element find error: %s", err)
	}
	err = element.element.Input(inputStr)
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

func (page *VPage) Close() error {
	return page.page.Close()
}
