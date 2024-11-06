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
	page *rod.Page
}

func (page *VPage) Navigate(urlStr string, waitFor string) error {
	var err error
	page.page = page.page.Timeout(30 * time.Second)
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

func (page *VPage) Element(selector string) (*rod.Element, error) {
	element, err := page.page.Element(selector)
	if err != nil {
		return nil, utils.Errorf("element find error: %s", err)
	}
	return element, nil
}

func (page *VPage) Elements(selector string) (rod.Elements, error) {
	elements, err := page.page.Elements(selector)
	if err != nil {
		return nil, utils.Errorf("elements find error: %s", err)
	}
	return elements, nil
}

func (page *VPage) Click(selector string) error {
	element, err := page.Element(selector)
	if err != nil {
		return utils.Errorf("click element find error: %s", err)
	}
	err = element.Click(proto.InputMouseButtonLeft, 1)
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

func (page *VPage) Close() error {
	return page.page.Close()
}
