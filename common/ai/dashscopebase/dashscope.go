package dashscopebase

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonpath"
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

func (d *DashScopeGateway) SupportedStructuredStream() bool {
	return true
}

func (d *DashScopeGateway) Chat(s string, function ...aispec.Function) (string, error) {
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

func (d *DashScopeGateway) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	return nil, utils.Error("not implemented: dashscope is not supported openai style chat ex")
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
			if data.OutputText != "" {
				pw.Write([]byte(data.OutputText))
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

func (d *DashScopeGateway) StructuredStream(s string, function ...aispec.Function) (chan *aispec.StructuredData, error) {
	var objChannel = make(chan *aispec.StructuredData, 1000)

	if d.dashscopeAPIKey == "" {
		return nil, utils.Error("APIKey is required")
	}

	go func() {
		defer func() {
			close(objChannel)
		}()
		count := 0
		rsp, req, err := poc.DoPOST(
			d.endpointUrl,
			poc.WithConnectTimeout(15),
			poc.WithTimeout(600),
			poc.WithReplaceHttpPacketHeader("Authorization", `Bearer `+d.dashscopeAPIKey),
			poc.WithReplaceHttpPacketHeader("X-DashScope-SSE", "enable"),
			poc.WithJSON(
				map[string]any{
					"input": map[string]any{
						"prompt": s,
					},
					"parameters": map[string]any{
						"stream":             true,
						"incremental_output": true,
						"has_thoughts":       true,
						"flow_stream_mode":   "agent_format",
					},
					"debug": map[string]any{},
				},
			),
			poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
				for {
					structured := &aispec.StructuredData{}
					for {
						resultBytes, err := utils.ReadLine(closer)
						if err != nil {
							log.Warnf("failed to read line in %v: %v", d.endpointUrl, err)
							return
						}
						result := string(resultBytes)
						if strings.TrimSpace(result) == "" {
							break
						}
						if strings.HasPrefix(result, "data:") {
							structured.DataRaw = bytes.TrimSpace([]byte(result[5:]))
							/*

								id:82
								event:result
								:HTTP_STATUS/200
								data:{"output":{"session_id":"8175fb34df364e2693bb99cb0f906c09","finish_reason":"null","text":"等具体场景"},"usage":{"models":[{"input_tokens":800,"output_tokens":5,"model_id":"qwen-max"},{"input_tokens":1574,"output_tokens":386,"model_id":"deepseek-r1"}]},"request_id":"fbfdb30e-30a8-9f84-9fe4-544330183e42"}

								id:83
								event:result
								:HTTP_STATUS/200
								data:{"output":{"session_id":"8175fb34df364e2693bb99cb0f906c09","finish_reason":"null","text":"切入学习"},"usage":{"models":[{"input_tokens":800,"output_tokens":5,"model_id":"qwen-max"},{"input_tokens":1574,"output_tokens":388,"model_id":"deepseek-r1"}]},"request_id":"fbfdb30e-30a8-9f84-9fe4-544330183e42"}

								id:84
								event:result
								:HTTP_STATUS/200
								data:{"output":{"session_id":"8175fb34df364e2693bb99cb0f906c09","finish_reason":"null","text":"。"},"usage":{"models":[{"input_tokens":800,"output_tokens":5,"model_id":"qwen-max"},{"input_tokens":1574,"output_tokens":389,"model_id":"deepseek-r1"}]},"request_id":"fbfdb30e-30a8-9f84-9fe4-544330183e42"}

							*/
							var i = make(map[string]any)
							if err := json.Unmarshal(structured.DataRaw, &i); err != nil {
								log.Warnf("failed to unmarshal data: %v", err)
								continue
							}
							structured.OutputText = utils.InterfaceToString(jsonpath.Find(i, "$.output.text"))
							structured.FinishedReason = utils.InterfaceToString(jsonpath.Find(i, "$.output.finish_reason"))
							if usage, ok := jsonpath.Find(i, "$.usage.models").([]any); ok && usage != nil {
								for _, u := range usage {
									if model, ok := u.(map[string]any); ok && model != nil {
										modelId, ok1 := model["model_id"]
										if !ok1 {
											continue
										}
										modelIdStr := utils.InterfaceToString(modelId)
										if modelIdStr == "" {
											continue
										}

										inputTokensRaw, ok2 := model["input_tokens"]
										if !ok2 {
											continue
										}
										outputTokensRaw, ok3 := model["output_tokens"]
										if !ok3 {
											continue
										}

										inputTokensStr := utils.InterfaceToString(inputTokensRaw)
										outputTokensStr := utils.InterfaceToString(outputTokensRaw)
										inputTokens, err1 := strconv.ParseInt(inputTokensStr, 10, 64)
										outputTokens, err2 := strconv.ParseInt(outputTokensStr, 10, 64)
										if err1 != nil || err2 != nil {
											log.Warnf("failed to parse tokens: %v, %v", err1, err2)
											continue
										}

										structured.UsageStats = append(structured.UsageStats, aispec.UsageStatsInfo{
											Model:       modelIdStr,
											InputToken:  int(inputTokens),
											OutputToken: int(outputTokens),
										})
									}
								}
							}
						} else if strings.HasSuffix(result, "id:") {
							structured.Id = strings.TrimSpace(result[3:])
						} else if strings.HasSuffix(result, "event:") {
							structured.Event = strings.TrimSpace(result[6:])
						}
					}
					if string(structured.DataRaw) == "" {
						continue
					}
					objChannel <- structured
					count++
				}
			}),
		)
		if count <= 2 {
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
	return objChannel, nil
}

func (d *DashScopeGateway) LoadOption(opt ...aispec.AIConfigOption) {
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
	return nil, nil
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
