package aispec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func ChatBase(url string, model string, msg string, fs []Function, opt func() ([]poc.PocConfigOption, error), streamHandler func(io.Reader)) (string, error) {
	opts, err := opt()
	if err != nil {
		return "", utils.Errorf("build config failed: %v", err)
	}
	msgIns := NewChatMessage(model, []ChatDetail{NewUserChatDetail(msg)}, fs...)

	handleStream := streamHandler != nil
	if handleStream {
		msgIns.Stream = true
	}

	raw, err := json.Marshal(msgIns)
	if err != nil {
		return "", utils.Errorf("build msg[%v] to json failed: %s", string(raw), err)
	}
	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))

	if streamHandler != nil {
		var pr io.Reader
		var body = bytes.NewBufferString("")
		pr, opts = appendStreamHandlerPoCOption(opts)
		wg := new(sync.WaitGroup)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Warnf("streamHandler panic: %v", err)
				}
			}()
			streamHandler(io.TeeReader(pr, body))
		}()
		_, _, err := poc.DoPOST(url, opts...)
		if err != nil {
			return "", utils.Errorf("request post to %v：%v", url, err)
		}
		wg.Wait()
		return body.String(), nil
	}

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

func ExtractFromResult(result string, fields map[string]any) (map[string]any, error) {
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sampleField := keys[0]

	stdjsons, raw := jsonextractor.ExtractJSONWithRaw(result)
	for _, stdjson := range stdjsons {
		var rawMap = make(map[string]any)
		err := json.Unmarshal([]byte(stdjson), &rawMap)
		if err != nil {
			fmt.Println(string(stdjson))
			log.Errorf("parse failed: %v", err)
			continue
		}
		_, ok := rawMap[sampleField]
		if ok {
			return rawMap, nil
		}
	}

	var err error
	for _, rawJson := range raw {
		stdjson := jsonextractor.FixJson([]byte(rawJson))
		var rawMap = make(map[string]any)
		err = json.Unmarshal([]byte(stdjson), &rawMap)
		if err != nil {
			fmt.Println(string(stdjson))
			log.Errorf("parse failed: %v", err)
			continue
		}
		_, ok := rawMap[sampleField]
		if ok {
			return rawMap, nil
		}
	}

	if strings.Contains(result, "，") {
		return ExtractFromResult(strings.ReplaceAll(result, "，", ","), fields)
	}

	return nil, utils.Errorf("cannot extractjson: \n%v\n", string(result))
}

func GenerateJSONPrompt(msg string, fields map[string]any) string {
	// 按字母序排列字段
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var fieldsDesc strings.Builder
	for i, k := range keys {
		fieldsDesc.WriteString(fmt.Sprintf("%d. 字段名：%#v, 含义：%#v;\n", i+1, k, fields[k]))
	}

	return `# 指令
你是一个专业的数据处理助手，请严格按以下要求处理输入内容：

## 处理步骤
1. 直接提取或总结所需数据
2. 必须使用JSON格式输出
3. 不要包含推理过程
4. 不要添加额外解释

## 输入内容
` + strconv.Quote(msg) + `

## 字段定义
` + fieldsDesc.String() + `

## 输出要求
- 使用严格JSON格式（无Markdown代码块）
- 确保类型正确：
* 数值类型：不要加引号
* 字符串类型：必须加双引号
* 空值返回null
- 示例格式：
{"field1":123,"field2":"text","field3":null}

请直接输出处理后的JSON：`
}

func ChatBasedExtractData(url string, model string, msg string, fields map[string]any, opt func() ([]poc.PocConfigOption, error), streamHandler func(io.Reader)) (map[string]any, error) {
	if len(fields) <= 0 {
		return nil, utils.Error("no fields config for extract")
	}

	if fields == nil || len(fields) <= 0 {
		fields = make(map[string]any)
		fields["raw_data"] = "相关数据"
	}
	msg = GenerateJSONPrompt(msg, fields)
	result, err := ChatBase(url, model, msg, nil, opt, streamHandler)
	if err != nil {
		log.Errorf("chatbase error: %s", err)
		return nil, err
	}
	result = strings.ReplaceAll(result, "`", "")
	return ExtractFromResult(result, fields)
}

func ChatExBase(url string, model string, details []ChatDetail, function []Function, opt func() ([]poc.PocConfigOption, error), streamHandler func(closer io.Reader)) ([]ChatChoice, error) {
	handleStream := streamHandler != nil
	opts, err := opt()
	if err != nil {
		return nil, err
	}
	msg := NewChatMessage(model, details, function...)
	if handleStream {
		msg.Stream = true
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, utils.Errorf("marshal message failed: %v", err)
	}
	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))

	if handleStream {
		var pr io.Reader
		var body = bytes.NewBufferString("")
		pr, opts = appendStreamHandlerPoCOption(opts)
		wg := new(sync.WaitGroup)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Warnf("streamHandler panic: %v", err)
				}
			}()
			streamHandler(io.TeeReader(pr, body))
		}()
		_, _, err := poc.DoPOST(url, opts...)
		if err != nil {
			return nil, utils.Errorf("request post to %v：%v", url, err)
		}
		wg.Wait()
		return []ChatChoice{
			{
				Index: 0,
				Message: ChatDetail{
					Role:    "system",
					Name:    "",
					Content: body.String(),
				},
				FinishReason: "stop",
			},
		}, nil
	}

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
	description string, paramRaw map[string]any,
	opt func() ([]poc.PocConfigOption, error),
	streamHandler func(io.Reader),
) (map[string]any, error) {
	parameters := &Parameters{
		Type:       "object",
		Properties: make(map[string]Property),
		Required:   make([]string, 0),
	}
	var requiredName []string
	for name, v := range paramRaw {
		parameters.Properties[name] = Property{
			Type: `string`, Description: codec.AnyToString(v),
		}
		requiredName = append(requiredName, name)
	}

	mainFunction := uuid.New().String()
	main := Function{
		Name:        mainFunction,
		Description: description,
		Parameters:  *parameters,
	}
	if main.Description == "" {
		main.Description = "extract and summary some useful info"
	}
	choice, err := ChatExBase(url, model, []ChatDetail{NewUserChatDetail(input)}, []Function{main}, opt, streamHandler)
	if err != nil {
		return nil, err
	}

	if choice == nil || len(choice) == 0 {
		return nil, utils.Error("no choice for chat result")
	}
	choiceMsg := choice[0].Message.Content
	if choiceMsg == "" {
		calls := choice[0].Message.ToolCalls
		if len(calls) > 0 {
			choiceMsg = calls[0].Function.Arguments
		}
	}
	if choiceMsg == "" {
		return nil, utils.Error("no choice message")
	}

	result := make(map[string]any)
	err = json.Unmarshal([]byte(choiceMsg), &result)
	if err != nil {
		results := jsonextractor.ExtractStandardJSON(choiceMsg)
		if len(results) > 0 {
			err = json.Unmarshal([]byte(results[0]), &result)
			if err != nil {
				return ChatBasedExtractData(url, model, input, result, opt, streamHandler)
				//return nil, utils.Errorf("unmarshal choice message[%v] failed: %v", string(choiceMsg), err)
			}
			return result, nil
		}
		return ChatBasedExtractData(url, model, input, paramRaw, opt, streamHandler)
	}
	return result, nil
}
