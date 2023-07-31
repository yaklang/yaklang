// Package httpbrute
// @Author bcy2007  2023/7/31 11:39
package httpbrute

import (
	"github.com/yaklang/yaklang/common/simulator/core"
	"testing"

	"github.com/go-rod/rod"
)

func TestElementSelectorGetFromDescription(t *testing.T) {
	// selector path: element.Object.Description
	urlStr := "http://testphp.vulnweb.com/"
	browser := rod.New().MustConnect()
	page := browser.MustPage(urlStr).MustWaitLoad()
	elements := page.MustElements("input")
	if len(elements) == 0 {
		t.Error(`input elements length 0.`)
	}
	for _, element := range elements {
		t.Logf(`%s`, element.Object.Description)
	}
}

func TestGetElementName(t *testing.T) {
	urlStr := "http://192.168.0.68/#/login"
	browser := rod.New().MustConnect()
	page := browser.MustPage(urlStr).MustWaitLoad()
	elements := page.MustElements("#code")
	for _, element := range elements {
		result, err := element.Eval(core.GETNAME)
		if err != nil {
			t.Errorf(err.Error())
			continue
		}
		t.Log(result.Value.Str())
	}
}
