package dashscopebase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/go-funk"

	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type DashScopeGateway struct {
	config *aispec.AIConfig

	dashscopeAppId  string
	dashscopeAPIKey string

	endpointUrl string
}

func (d *DashScopeGateway) GetModelList() ([]*aispec.ModelMeta, error) {
	return nil, nil
}

func (d *DashScopeGateway) SupportedStructuredStream() bool {
	return true
}

func (d *DashScopeGateway) Chat(s string, function ...any) (string, error) {
	reader, err := d.ChatStream(s)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	if d.config != nil {
		if d.config.StreamHandler != nil {
			teeReader := io.TeeReader(reader, &buf)
			d.config.StreamHandler(teeReader)
			return buf.String(), nil
		}
	}
	io.Copy(&buf, reader)
	return buf.String(), nil
}

func (d *DashScopeGateway) ChatStream(s string) (io.Reader, error) {
	ch, err := d.StructuredStream(s)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for data := range ch {
			if len(data.DataRaw) > 0 {
				pw.Write(data.DataRaw)
				pw.Write([]byte("\n"))
			}
		}
	}()
	return pr, nil
}

func (d *DashScopeGateway) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	prompt := aispec.GenerateJSONPrompt(data+"\n"+desc, fields)
	result, err := d.Chat(prompt)
	if err != nil && result == "" {
		return nil, err
	}
	return aispec.ExtractFromResult(result, fields)
}

type rspFeeder struct {
	visited sync.Map
}

func (d *rspFeeder) GetNewData(structued *aispec.StructuredData, input []byte) chan *aispec.StructuredData {
	var resultChan = make(chan *aispec.StructuredData, 1000)
	go func() {
		defer close(resultChan)
		// {"output":{"thoughts":[{"response":"{\"nodeName\":\"开始\",\"nodeType\":\"Start\",\"nodeStatus\":\"success\",\"nodeId\":\"Start_bYxoRU\",\"nodeExecTime\":\"0ms\"}"},{"response":"{\"nodeName\":\"prerag\",\"nodeResult\":\"{\\\"result\\\":\\\"\\\"}\",\"nodeType\":\"LLM\",\"nodeStatus\":\"success\",\"nodeId\":\"LLM_j4AP\",\"nodeExecTime\":\"2638ms\"}"},{"response":"{\"nodeName\":\"yaklang-rag\",\"nodeResult\":\"{\\\"result\\\":\\\"\\\"}\",\"nodeType\":\"Retrieval\",\"nodeStatus\":\"success\",\"nodeId\":\"Retrieval_hK8d\",\"nodeExecTime\":\"765ms\"}"},{"response":"{\"nodeName\":\"code-generator\",\"nodeResult\":\"{\\\"result\\\":\\\"\\\",\\\"reasoningContent\\\":\\\"好的，用户想要在Y\\\"}\",\"nodeType\":\"LLM\",\"nodeStatus\":\"executing\",\"nodeId\":\"LLM_pBed\"}"}],"session_id":"3daa9655f7614515a08cda59d26ccaa7","finish_reason":"null"},"usage":{"models":[{"input_tokens":809,"output_tokens":49,"model_id":"qwen-max"},{"input_tokens":1464,"output_tokens":8,"model_id":"deepseek-r1"}]},"request_id":"db3c29e3-c696-9de3-903b-3886f87b4404"}
		var data = make(map[string]any)
		err := json.Unmarshal(input, &data)
		if err != nil {
			return
		}

		usages := make(map[string]aispec.UsageStatsInfo)
		if usageRaw, ok := data["usage"]; ok {
			if usage, ok := usageRaw.(map[string]any); ok {
				if models, ok := usage["models"]; ok {
					// "usage":{"models":[{"input_tokens":809,"output_tokens":49,"model_id":"qwen-max"},{"input_tokens":1464,"output_tokens":8,"model_id":"deepseek-r1"}]}
					if funk.IsIteratee(models) {
						funk.ForEach(models, func(i any) {
							structued.HaveUsage = true
							if model, ok := i.(map[string]any); ok {
								if modelId, ok := model["model_id"]; ok {
									inputTokens := 0
									outputTokens := 0
									if inputTokensRaw, ok := model["input_tokens"]; ok {
										inputTokens = int(inputTokensRaw.(float64))
									}
									if outputTokensRaw, ok := model["output_tokens"]; ok {
										outputTokens = int(outputTokensRaw.(float64))
									}
									usageInstance := aispec.UsageStatsInfo{
										Model:       modelId.(string),
										InputToken:  inputTokens,
										OutputToken: outputTokens,
									}
									usages[fmt.Sprint(modelId)] = usageInstance
								}
							}
						})
					}
				}
			}
		}

		if outputRaw, ok := data["output"]; ok {
			if output, ok := outputRaw.(map[string]any); ok {
				if thoughts, ok := output["thoughts"]; ok {
					if !funk.IsIteratee(thoughts) {
						structued.IsParsed = false
						return
					}

					funk.ForEach(thoughts, func(i any) {
						thought := utils.InterfaceToGeneralMap(i)
						if responseRaw, ok := thought["response"]; ok {
							var response = make(map[string]any)
							responseStr := fmt.Sprint(responseRaw)
							hash := utils.CalcSha256(responseStr)
							if _, ok := d.visited.Load(hash); ok {
								return
							}
							d.visited.Store(hash, structued)
							err := json.Unmarshal([]byte(responseStr), &response)
							if err != nil {
								return
							}

							newStructued := structued.Copy()

							// "{\"nodeName\":\"开始\",\"nodeType\":\"Start\",\"nodeStatus\":\"success\",\"nodeId\":\"Start_bYxoRU\",\"nodeExecTime\":\"0ms\"}"
							newStructued.OutputNodeId = fmt.Sprint(response["nodeId"])
							newStructued.OutputNodeName = fmt.Sprint(response["nodeName"])
							newStructued.OutputNodeType = fmt.Sprint(response["nodeType"])
							newStructued.OutputNodeStatus = fmt.Sprint(response["nodeStatus"])
							if response["nodeExecTime"] != nil {
								newStructued.OutputNodeExecTime = fmt.Sprint(response["nodeExecTime"])
							}
							nodeResult, ok := response["nodeResult"]
							if ok {
								var result = make(map[string]any)
								_ = json.Unmarshal([]byte(nodeResult.(string)), &result)
								if resultText, ok := result["result"]; ok {
									newStructued.OutputText = fmt.Sprint(resultText)
								}
								if resultReason, ok := result["reasoningContent"]; ok {
									newStructued.OutputReason = fmt.Sprint(resultReason)
								}
							}
							newStructued.IsParsed = true
							for _, usage := range usages {
								newStructued.ModelUsage = append(newStructued.ModelUsage, usage)
							}
							resultChan <- newStructued
						}
					})
				}
			}
		}
	}()
	return resultChan
}

func (d *DashScopeGateway) StructuredStream(s string, function ...any) (chan *aispec.StructuredData, error) {
	var objChannel = make(chan *aispec.StructuredData, 1000)

	if d.dashscopeAPIKey == "" {
		return nil, utils.Error("APIKey is required")
	}

	go func() {
		defer close(objChannel)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("StructuredStream panic: %v", utils.ErrorStack(err))
			}
		}()
		feeder := &rspFeeder{}
		reader, writer := utils.NewPipe()
		var count = new(int64)
		addCount := func() {
			atomic.AddInt64(count, 1)
		}
		getCount := func() int64 {
			return atomic.LoadInt64(count)
		}
		go func() {
			opts, _ := d.BuildHTTPOptions()
			opts = append(opts, poc.WithTimeout(600))
			opts = append(opts, poc.WithReplaceHttpPacketHeader("Authorization", `Bearer `+d.dashscopeAPIKey))
			opts = append(opts, poc.WithReplaceHttpPacketHeader("X-DashScope-SSE", "enable"))
			opts = append(opts, poc.WithJSON(
				map[string]any{
					"input": map[string]any{
						"prompt": s,
					},
					"parameters": map[string]any{
						"stream":             false,
						"incremental_output": true,
						"has_thoughts":       true,
						// "flow_stream_mode":   "agent_format",
					},
					"debug": map[string]any{},
				},
			))
			opts = append(opts, poc.WithBodyStreamReaderHandler(func(r []byte, rawCloser io.ReadCloser) {
				chunked := strings.ToLower(strings.TrimSpace(lowhttp.GetHTTPPacketHeader(r, "transfer-encoding"))) == "chunked"
				var bodyReader io.Reader = rawCloser
				if chunked {
					log.Infof("SSE Chunked for: %v", d.endpointUrl)
					bodyReader = httputil.NewChunkedReader(rawCloser)
				}
				defer func() {
					writer.Close()
				}()
				io.Copy(writer, bodyReader)
			}))
			rsp, req, err := poc.DoPOST(d.endpointUrl, opts...)
			if getCount() <= 2 {
				if rsp != nil && rsp.RawPacket != nil && len(rsp.RawPacket) > 0 {
					log.Infof(" request: \n%v", string(rsp.RawRequest))
					log.Infof("response: \n%v", string(rsp.RawPacket))
					log.Errorf("failed to do post, body: %v", string(rsp.GetBody()))
				}
			}
			if err != nil {
				log.Warnf("failed to do post: %v", err)
				if req != nil {
					reqRaw, _ := utils.DumpHTTPRequest(req, true)
					if len(reqRaw) > 0 {
						log.Warnf("request: \n%s", string(reqRaw))
					}
				}
				return
			}
			_ = rsp
		}()
		for {
			structured := &aispec.StructuredData{
				DataSourceType: "dashscope",
			}
			handleLine := func(line string) {
				if strings.HasPrefix(line, "data:") {
					structured.DataRaw = bytes.TrimSpace([]byte(line[5:]))
					inputs := []byte(line[5:])
					ch := feeder.GetNewData(structured, inputs)
					if ch != nil {
						for data := range ch {
							// 检查退出信号
							select {
							case objChannel <- data:
								addCount()
							}
						}
					} else {
						if len(structured.DataRaw) > 0 {
							select {
							case objChannel <- structured:
								addCount()
							}
						}
					}
				} else if strings.HasPrefix(line, "id:") {
					structured.Id = strings.TrimSpace(line[3:])
				} else if strings.HasPrefix(line, "event:") {
					structured.Event = strings.TrimSpace(line[6:])
				}
			}
			resultBytes, err := utils.ReadLine(reader)
			if err != nil && len(resultBytes) <= 0 {
				if err != io.EOF {
					log.Warnf("failed to read line in %v: %v", d.endpointUrl, err)
				}
				return
			}
			result := string(resultBytes)
			handleLine(result)
		}
	}()
	return objChannel, nil
}

func (d *DashScopeGateway) newLoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	d.config = config
	d.dashscopeAPIKey = config.APIKey

	d.endpointUrl = aispec.GetBaseURLFromConfigEx(d.config, "https://dashscope.aliyuncs.com", "/api/v1/apps/"+strings.TrimSpace(d.dashscopeAppId)+"/completion", false)
}

func (d *DashScopeGateway) LoadOption(opt ...aispec.AIConfigOption) {
	if aispec.EnableNewLoadOption {
		d.newLoadOption(opt...)
		return
	}
	config := aispec.NewDefaultAIConfig(opt...)
	urlStr := `https://dashscope.aliyuncs.com/api/v1/apps/` + strings.TrimSpace(d.dashscopeAppId) + `/completion`
	if config.BaseURL != "" {
		urlStr = config.BaseURL
	}

	if utils.IsHttpOrHttpsUrl(config.Domain) {
		urlStr = config.Domain
	} else if config.Domain != "" {
		urlIns, _ := url.Parse(urlStr)
		if urlIns != nil {
			urlIns.Host = config.Domain
		}
		urlStr = urlIns.String()
	}
	d.endpointUrl = urlStr
	d.config = config
	d.dashscopeAPIKey = config.APIKey
}

func (d *DashScopeGateway) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	var opts []poc.PocConfigOption
	if d.config.Context == nil {
		d.config.Context = context.Background()
		opts = append(opts, poc.WithContext(d.config.Context))
	}
	if d.config.Timeout > 0 {
		opts = append(opts, poc.WithConnectTimeout(d.config.Timeout))
	}
	if d.config.Host != "" {
		opts = append(opts, poc.WithHost(d.config.Host))
	}
	if d.config.Port > 0 {
		opts = append(opts, poc.WithPort(d.config.Port))
	}
	return opts, nil
}

func (d *DashScopeGateway) CheckValid() error {
	if d.dashscopeAppId == "" {
		return utils.Error("AppId is required")
	}
	if d.dashscopeAPIKey == "" {
		return utils.Errorf("APIKey is required")
	}
	return nil
}

var _ aispec.AIClient = &DashScopeGateway{}
