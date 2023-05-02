package core

import (
	"strings"
	"yaklang/common/log"
	"yaklang/common/rpa/captcha"
	"yaklang/common/utils"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

func (m *Manager) extractInput(page *rod.Page) error {
	// get input element
	eles, err := page.Elements("input")
	if err != nil {
		return utils.Errorf("element input get error:%s", err)
	}
	for _, ele := range eles {

		ele_type, err := ele.Attribute("type")
		if err != nil {
			// return utils.Errorf("get input element type error: %s", err)
			if m.detailLog {
				log.Errorf("get input element type error: %s", err)
			}
			continue
		}
		// meet submit checkbox to skip because of it needs to be click in other function
		// hidden need to be skipped because of click errors
		if ele_type != nil {
			if *ele_type == "submit" || *ele_type == "checkbox" || *ele_type == "hidden" {
				continue
			}
		}

		// dont input value when value exist
		value, err := ele.Attribute("value")
		if err != nil {
			// return utils.Errorf("get input element value error: %s", err)
			if m.detailLog {
				log.Errorf("get input element value error: %s", err)
			}
			continue
		}
		if value != nil && *value != "" {
			continue
		}

		// get name & placeholder to know its type and get input informations from type
		name, err := ele.Attribute("name")
		if err != nil {
			// return utils.Errorf("get input element name error: %s", err)
			if m.detailLog {
				log.Errorf("get input element name error: %s", err)
			}
			continue
		}
		placeholder, err := ele.Attribute("placeholder")
		if err != nil {
			// return utils.Errorf("get input element placeholder error: %s", err)
			if m.detailLog {
				log.Errorf("get input element placeholder error: %s", err)
			}
			continue
		}

		var ele_input, ele_kw string
		if name != nil {
			ele_kw = GetKeywordType(*name)
		}
		if ele_kw == "" && placeholder != nil {
			ele_kw = GetKeywordType(*placeholder)
		}
		if ele_kw == "captcha" {
			capt := &captcha.Captcha{
				Feature_element: ele,
				Domain:          m.mainDomain,
				CaptchaUrl:      m.captchaUrl,
			}
			ele_input, err = capt.GetCaptcha()
			if err != nil {
				if m.detailLog {
					log.Errorf("get captcha error:%s", err)
				}
				ele_input = "aaaa"
			}
		} else {
			var ok bool
			ele_input, ok = defaultFillForm[ele_kw]
			if !ok {
				ele_input = "bbb"
			}
		}
		err = m.ElementInputWords(ele, ele_input)
		if err != nil {
			// keyboard input error and use *element.input
			// hard to run
			log.Info("element input words err:%s", err)
			ele.Input(ele_input)
		}
	}
	return nil
}

// from name & placehold to know input information type like username, password etc.
func GetKeywordType(kw string) string {
	for k, v := range getDefault {
		for _, item := range v {
			if strings.Contains(kw, item) {
				return k
			}
		}
	}
	return ""
}

// input by keyboard
func (m *Manager) ElementInputWords(element *rod.Element, words string) error {
	runeStr := []input.Key(words)
	element.Eval(`()=>this.click()`)
	err := element.Type(runeStr...)
	if err != nil {
		return utils.Errorf("element keyboard input words error: %s", err)
	}
	return nil
}
