package core

import (
	"encoding/json"
	"yaklang.io/yaklang/common/utils"

	"github.com/go-rod/rod"
)

const GETURL = `
()=>{
	return document.URL;
}
`

const GETREADYSTATE = `
()=>{
	return document.readyState;
}
`

// get current page url by js
func (m *Manager) GetCurrentUrl(page *rod.Page) (url string, err error) {
	result, err := page.Eval(GETURL)
	if err != nil {
		return "", utils.Errorf("eval get current url error: %s", err)
	}
	raw, err := result.Value.MarshalJSON()
	if err != nil {
		return "", utils.Errorf("marshal json error: %s from %s", err, result)
	}
	var str string
	err = json.Unmarshal(raw, &str)
	if err != nil {
		return "", utils.Errorf("json unmarshal error: %s from %s", err, raw)
	}
	return str, nil
}

// get page state by js
func (m *Manager) GetReadyState(page *rod.Page) (state string, err error) {
	result, err := page.Eval(GETREADYSTATE)
	if err != nil {
		return "", utils.Errorf("eval get readystate error: %s", err)
	}
	raw, err := result.Value.MarshalJSON()
	if err != nil {
		return "", utils.Errorf("marshal json error: %s from %s", err, result)
	}
	var str string
	err = json.Unmarshal(raw, &str)
	if err != nil {
		return "", utils.Errorf("json unmarshal error: %s from %s", err, raw)
	}
	return str, nil
}
