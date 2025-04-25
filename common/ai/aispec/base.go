package aispec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/davecgh/go-spew/spew"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func ListChatModels(url string, opt func() ([]poc.PocConfigOption, error)) ([]*ModelMeta, error) {
	opts, err := opt()
	if err != nil {
		return nil, utils.Errorf("build config failed: %v", err)
	}
	opts = append(opts, poc.WithTimeout(600), poc.WithConnectTimeout(8), poc.WithRetryTimes(3))

	if strings.HasSuffix(url, "/") {
		// remove /
		url = url[:len(url)-1]
	}
	if strings.HasSuffix(url, "/chat/completions") {
		// remove /chat/completions
		url = url[:len(url)-len("/chat/completions")]
		url += "/models"
	}

	log.Infof("requtest GET to %v to find available models", url)
	rsp, _, err := poc.DoGET(url, opts...)
	if err != nil {
		return nil, utils.Errorf("request get to %v：%v", url, err)
	}

	// body like  {"object":"list","data":[{"id":"qwq:latest","object":"model","created":1741877931,"owned_by":"library"},{"id":"gemma3:27b","object":"model","created":1741875247,"owned_by":"library"},{"id":"deepseek-r1:32b","object":"model","created":1738946811,"owned_by":"library"},{"id":"deepseek-r1:70b","object":"model","created":1738939603,"owned_by":"library"},{"id":"qwen2.5:32b","object":"model","created":1727615210,"owned_by":"library"},{"id":"qwen2.5:latest","object":"model","created":1727613786,"owned_by":"library"}]}
	body := rsp.GetBody()
	if len(body) <= 0 {
		return nil, utils.Errorf("empty response")
	}

	var resp struct {
		Object string       `json:"object"`
		Data   []*ModelMeta `json:"data"`
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		return nil, utils.Errorf("unmarshal models failed: %v raw:\n%v", err, spew.Sdump(body))
	}

	return resp.Data, nil
}

type streamToStructuredStream struct {
	isReason bool
	id       func() int
	idInc    func()
	mutex    *sync.Mutex
	r        chan *StructuredData
}

func (s *streamToStructuredStream) Write(p []byte) (n int, err error) {
	if s.r == nil {
		return 0, utils.Error("streamToStructuredStream is not initialized")
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.idInc != nil {
		s.idInc()
	}
	id := s.id()

	data := &StructuredData{
		Id:           fmt.Sprint(id),
		Event:        "data",
		OutputText:   "",
		OutputReason: "",
	}
	if s.isReason {
		data.OutputReason = string(p)
	} else {
		data.OutputText = string(p)
	}
	s.r <- data
	return len(p), nil
}

func StructuredStreamBase(
	url string,
	model string,
	msg string,
	opt func() ([]poc.PocConfigOption, error),
	streamHandler func(io.Reader),
	reasonHandler func(io.Reader),
	errHandler func(error),
) (chan *StructuredData, error) {
	var schan = make(chan *StructuredData, 1000)
	id := 0
	getId := func() int {
		return id
	}
	idInc := func() {
		id++
	}
	m := new(sync.Mutex)
	go func() {
		_, err := ChatBase(url, model, msg, nil, opt, func(reader io.Reader) {
			structured := &streamToStructuredStream{
				isReason: false,
				id:       getId,
				idInc:    idInc,
				mutex:    m,
				r:        schan,
			}
			if streamHandler == nil {
				// read from reader
				io.Copy(structured, reader)
				return
			}
			// tee reader to mirror streamHandler
			r, w := utils.NewPipe()
			defer w.Close()
			newReader := io.TeeReader(reader, w)
			go func() { streamHandler(r) }()

			// read from newReader
			io.Copy(structured, newReader)
		}, func(reader io.Reader) {
			structured := &streamToStructuredStream{
				isReason: true,
				id:       getId,
				idInc:    idInc,
				mutex:    m,
				r:        schan,
			}
			if reasonHandler == nil {
				io.Copy(structured, reader)
				return
			}
			// tee reader to mirror streamHandler
			r, w := utils.NewPipe()
			defer w.Close()
			newReader := io.TeeReader(reader, w)
			go func() { streamHandler(r) }()
			// read from newReader
			io.Copy(structured, newReader)
		}, errHandler)
		if err != nil {
			log.Errorf("structured stream error: %v", err)
		}
	}()
	return schan, nil
}

func ChatBase(
	url string, model string, msg string, fs []Function, opt func() ([]poc.PocConfigOption, error),
	streamHandler func(io.Reader), reasonStreamHandler func(reader io.Reader),
	errHandler func(error),
) (string, error) {
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
	opts = append(opts, poc.WithConnectTimeout(5))
	opts = append(opts, poc.WithRetryTimes(3))

	var pr, reasonPr io.Reader
	var cancel context.CancelFunc
	pr, reasonPr, opts, cancel = appendStreamHandlerPoCOptionEx(opts)
	wg := new(sync.WaitGroup)

	noMerge := false
	// handle out and reason
	if reasonStreamHandler != nil {
		noMerge = true
		// reason is not empty, not merge output
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Warnf("reasonStreamHandler panic: %v", err)
				}
			}()
			reasonStreamHandler(reasonPr)
		}()
	}

	if streamHandler != nil {
		var body = bytes.NewBufferString("")
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Warnf("streamHandler panic: %v", err)
				}
			}()
			if streamHandler != nil && noMerge {
				streamHandler(io.TeeReader(pr, body))
			} else {
				result := mergeReasonIntoOutputStream(reasonPr, pr)
				streamHandler(io.TeeReader(result, body))
			}
		}()
		rsp, _, err := poc.DoPOST(url, opts...)
		_ = rsp
		if err != nil {
			if errHandler != nil {
				errHandler(err)
			}
			if !utils.IsNil(cancel) {
				cancel()
			}
			wg.Wait()
			return "", utils.Errorf("request post to %v：%v", url, err)
		}
		wg.Wait()
		return body.String(), nil
	}

	_, _, err = poc.DoPOST(url, opts...)
	if err != nil {
		if errHandler != nil {
			errHandler(err)
		}
		return "", utils.Errorf("request post to %v：%v", url, err)
	}

	reader := mergeReasonIntoOutputStream(reasonPr, pr)
	bodyRaw, err := io.ReadAll(reader)
	return string(bodyRaw), nil
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

func ChatBasedExtractData(
	url string, model string, msg string, fields map[string]any, opt func() ([]poc.PocConfigOption, error),
	streamHandler func(io.Reader),
	reasonHandler func(io.Reader),
	httpErrorHandler func(error),
) (map[string]any, error) {
	if len(fields) <= 0 {
		return nil, utils.Error("no fields config for extract")
	}

	if fields == nil || len(fields) <= 0 {
		fields = make(map[string]any)
		fields["raw_data"] = "相关数据"
	}
	msg = GenerateJSONPrompt(msg, fields)
	result, err := ChatBase(url, model, msg, nil, opt, streamHandler, reasonHandler, httpErrorHandler)
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
	opts = append(opts, poc.WithConnectTimeout(10))
	opts = append(opts, poc.WithRetryTimes(3))

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
