package dashscopebase

import (
	"bytes"
	"io"
	"net/url"
	"strconv"
	"strings"

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
						"stream":             false,
						"incremental_output": true,
						"has_thoughts":       true,
						// "flow_stream_mode":   "agent_format",
					},
					"debug": map[string]any{},
				},
			),
			poc.WithBodyStreamReaderHandler(func(r []byte, closer io.ReadCloser) {
				chunked := strings.ToLower(strings.TrimSpace(lowhttp.GetHTTPPacketHeader(r, "transfer-encoding"))) == "chunked"
				if chunked {
					log.Infof("SSE Chunked for: %v", d.endpointUrl)
				}
				for {
					structured := &aispec.StructuredData{
						DataSourceType: "dashscope",
					}
					handleLine := func(line string) {
						if strings.HasPrefix(line, "data:") {
							structured.DataRaw = bytes.TrimSpace([]byte(line[5:]))
						} else if strings.HasPrefix(line, "id:") {
							structured.Id = strings.TrimSpace(line[3:])
						} else if strings.HasPrefix(line, "event:") {
							structured.Event = strings.TrimSpace(line[6:])
						}
					}
					for {
						resultBytes, err := utils.ReadLine(closer)
						if err != nil {
							log.Warnf("failed to read line in %v: %v", d.endpointUrl, err)
							return
						}
						result := string(resultBytes)
						size, _ := strconv.ParseInt(result, 16, 64)
						if chunked {
							if size > 0 {
								var buf = make([]byte, size)
								io.ReadFull(closer, buf)
								for _, line := range utils.ParseStringToLines(string(buf)) {
									handleLine(line)
								}
							}
							_, _ = utils.ReadLine(closer)
							break
						}
						if strings.TrimSpace(result) == "" {
							break
						}
						handleLine(result)
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
