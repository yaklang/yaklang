// Package crawlerx
// @Author bcy2007  2024/4/2 14:44
package preaction

import (
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

type ActionType string

const (
	HoverAction   ActionType = "hover"
	InputAction   ActionType = "input"
	ClickAction   ActionType = "click"
	SelectAction  ActionType = "select"
	SetFileAction ActionType = "setFile"
)

type PreAction struct {
	Action   ActionType
	Selector string
	Params   string
}

type PreActionJson struct {
	Action   string `json:"action,omitempty"`
	Selector string `json:"selector,omitempty"`
	Params   string `json:"params,omitempty"`
}

func (action PreAction) String() string {
	actionStr := ""
	actionStr = fmt.Sprintf("%v element %v", action.Action, action.Selector)
	if action.Params != "" {
		actionStr += fmt.Sprintf(" with %v", action.Params)
	}
	return actionStr
}

func PreAct(page *rod.Page, action *PreAction) error {
	elements, err := page.Elements(action.Selector)
	if err != nil {
		return utils.Errorf("find pre action element error: %v", err)
	}
	if len(elements) == 0 {
		return utils.Errorf("cannot find %v element", action.Selector)
	}
	element := elements.First()
	visible, err := element.Visible()
	if !visible || err != nil {
		return utils.Errorf("pre action element visible %v with error: %v", visible, err)
	}
	switch action.Action {
	case HoverAction:
		err = element.Hover()
	case InputAction:
		err = element.Input(action.Params)
	case ClickAction:
		err = element.Click(proto.InputMouseButtonLeft, 1)
	case SelectAction:
		err = element.Select([]string{action.Params}, true, rod.SelectorTypeText)
	case SetFileAction:
		err = element.SetFiles([]string{action.Params})
	default:
		return utils.Error("invalid pre action type")
	}
	if err != nil {
		return utils.Errorf("pre action %s error: %v", action, err)
	}
	err = page.WaitLoad()
	if err != nil {
		return utils.Errorf("pre action %s page wait load error: %v", action, err)
	}
	//time.Sleep(500 * time.Millisecond)
	time.Sleep(time.Second)
	return nil
}

func PreActs(page *rod.Page, actions []*PreAction) error {
	for _, action := range actions {
		err := PreAct(page, action)
		if err != nil {
			return utils.Errorf("pre actions error: %v", err)
		}
	}
	return nil
}
