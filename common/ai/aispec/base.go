package aispec

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func ChatBase(url string, model string, msg string, fs []Function, opt func() ([]poc.PocConfigOption, error)) (string, error) {
	opts, err := opt()
	if err != nil {
		return "", utils.Errorf("build config failed: %v", err)
	}
	msgIns := NewChatMessage(model, []ChatDetail{NewUserChatDetail(msg)})

	raw, err := json.Marshal(msgIns)
	if err != nil {
		return "", utils.Errorf("build msg[%v] to json failed: %s", string(raw), err)
	}
	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))
	rsp, _, err := poc.DoPOST(url, opts...)
	if err != nil {
		return "", utils.Errorf("request post to %v：%v", url, err)
	}
	var compl ChatCompletion
	err = json.Unmarshal(rsp.GetBody(), &compl)
	if err != nil || len(compl.Choices) == 0 {
		return "", utils.Errorf("JSON response (%v) failed：%v", string(rsp.GetBody()), err)
	}
	return compl.Choices[0].Message.Content, nil
}

func ChatExBase(url string, model string, details []ChatDetail, function []Function, opt func() ([]poc.PocConfigOption, error)) ([]ChatChoice, error) {
	opts, err := opt()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(NewChatMessage(model, details, function...))
	if err != nil {
		return nil, utils.Errorf("marshal message failed: %v", err)
	}
	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))
	rsp, _, err := poc.DoPOST(url, opts...)
	if err != nil {
		return nil, utils.Errorf("request post to %v：%v", url, err)
	}
	var compl ChatCompletion
	err = json.Unmarshal(rsp.GetBody(), &compl)
	if err != nil {
		return nil, utils.Errorf("JSON response (%v) failed：%v", string(rsp.GetBody()), err)
	}
	return compl.Choices, nil
}

func ExtractDataBase(
	url string, model string, input string,
	description string, param map[string]string,
	opt func() ([]poc.PocConfigOption, error),
) (map[string]any, error) {
	parameters := &Parameters{
		Type:       "object",
		Properties: make(map[string]Property),
		Required:   make([]string, 0),
	}
	var requiredName []string
	for name, v := range param {
		parameters.Properties[name] = Property{
			Type: `string`, Description: v,
		}
		requiredName = append(requiredName, name)
	}

	mainFunction := uuid.New().String()
	main := Function{
		Name:        mainFunction,
		Description: description,
		Parameters:  *parameters,
	}
	choice, err := ChatExBase(url, model, []ChatDetail{NewUserChatDetail(input)}, []Function{main}, opt)
	if err != nil {
		return nil, err
	}
	if choice == nil || len(choice) == 0 {
		return nil, utils.Error("no choice for chat result")
	}
	choiceMsg := choice[0].Message.Content
	result := make(map[string]any)
	err = json.Unmarshal([]byte(choiceMsg), &result)
	if err != nil {
		return nil, utils.Errorf("unmarshal choice message[%v] failed: %v", string(choiceMsg), err)
	}
	return result, nil
}
