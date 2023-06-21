package yakgrpc

import (
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

type sender interface {
	Send(*ypb.ExecResult) error
	Context() context.Context
}

func (s *Server) execScript(scriptName string, input string, stream sender, params ...*ypb.HTTPRequestBuilderParams) error {
	var (
		targetInput = input
	)
	if targetInput == "" {
		return utils.Error("input is empty")
	}

	if scriptName == "" {
		return utils.Error("code N scriptName is empty")
	}

	var builderParams *ypb.HTTPRequestBuilderParams
	if len(params) > 0 {
		builderParams = params[0]
	}

	if builderParams == nil {
		builderParams = &ypb.HTTPRequestBuilderParams{
			Method: "GET",
			Path:   []string{"/"},
		}
	}

	scriptInstance, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), scriptName)
	if err != nil {
		return err
	}
	var (
		debugType = scriptInstance.Type
		isTemp    = scriptInstance.Ignored && strings.HasPrefix(scriptInstance.ScriptName, "tmp-")
	)

	switch strings.ToLower(debugType) {
	case "mitm":
		fallthrough
	case "port-scan":
		fallthrough
	case "nuclei":
	default:
		return utils.Error("unsupported plugin type: " + debugType)
	}

	var builderResponse, _ = s.HTTPRequestBuilder(stream.Context(), builderParams)

	var reqs []any
	feed := func(req []byte, isHttps bool) {
		reqs = append(reqs, map[string]any{
			"RawHTTPRequest": req,
			"IsHttps":        isHttps,
		})
	}
	var results = builderResponse.GetResults()
	if len(results) <= 0 {
		var templates = []byte("GET / HTTP/1.1\r\nHost: {{Hostname}}\r\n\r\n")
		for _, res := range utils.PrettifyListFromStringSplitEx(targetInput, "\n", "|", ",") {
			res = strings.TrimSpace(res)
			if strings.HasPrefix(res, "http://") || strings.HasPrefix(res, "https://") {
				isHttps, raw, err := lowhttp.ParseUrlToHttpRequestRaw("GET", res)
				if err == nil {
					feed(raw, isHttps)
				}
				continue
			}

			host, port, _ := utils.ParseStringToHostPort(res)
			if host == "" {
				host = res
			}
			if host == "" {
				continue
			}
			if port > 0 {
				if port == 443 {
					feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(host)), true)
					continue
				}

				if port == 80 {
					feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(host)), false)
					continue
				}

				feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(utils.HostPort(host, port))), strings.HasPrefix(res, "https://"))
			} else {
				feed(bytes.ReplaceAll(templates, []byte(`{{Hostname}}`), []byte(host)), strings.HasPrefix(res, "https://"))
			}
		}
	} else {
		funk.ForEach(builderResponse.GetResults(), func(i *ypb.HTTPRequestBuilderResult) {
			for _, res := range utils.PrettifyListFromStringSplitEx(targetInput, "\n", "|", ",") {
				res = strings.TrimSpace(res)
				host, port, _ := utils.ParseStringToHostPort(res)
				if host == "" {
					host = res
				}
				if host == "" {
					continue
				}
				var handledRaw = bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(utils.HostPort(host, port)))
				if strings.HasPrefix(res, "http://") || strings.HasPrefix(res, "https://") {
					https := strings.HasPrefix(res, "https://")
					feed(lowhttp.UrlToGetRequestPacket(res, handledRaw, https), https)
					continue
				}

				if port > 0 {
					if i.GetIsHttps() && port == 443 {
						feed(bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(host)), i.GetIsHttps())
						continue
					}

					if !i.GetIsHttps() && port == 80 {
						feed(bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(host)), i.GetIsHttps())
						continue
					}

					feed(handledRaw, i.GetIsHttps())
				} else {
					feed(bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(host)), i.GetIsHttps())
				}
			}
		})
	}
	if len(reqs) <= 0 {
		return utils.Error("build http request failed: no results")
	}

	defer func() {
		if err := recover(); err != nil {
			log.Warn(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}

		if isTemp {
			log.Infof("start to remove temp plugin: %v", scriptName)
			err = yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), scriptName)
			if err != nil {
				log.Errorf("remove temp plugin failed: %v", err)
			}
		}
	}()

	// 不同的插件类型，需要不同的处理
	switch strings.ToLower(debugType) {
	case "mitm":
	case "nuclei":
	case "port-scan":
	default:
		return utils.Error("unsupported plugin type: " + debugType)
	}

	var feedbackClient = yaklib.NewVirtualYakitClientWithExecResult(stream.Send)
	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)

	log.Infof("engine.ExecuteExWithContext(stream.Context(), debugScript ... \n")
	subEngine, err := engine.ExecuteExWithContext(stream.Context(), debugScript, map[string]any{
		"REQUESTS":    reqs,
		"CTX":         stream.Context(),
		"PLUGIN_NAME": scriptName,
	})
	if err != nil {
		log.Warnf("execute debug script failed: %v", err)
		return err
	}
	_ = subEngine

	return nil
}

func (s *Server) debugScript(
	input string,
	debugType string,
	debugCode string,
	stream sender,
	params ...*ypb.HTTPRequestBuilderParams) error {
	tempName, err := yakit.CreateTemporaryYakScript(debugType, debugCode)
	if err != nil {
		return err
	}
	return s.execScript(tempName, input, stream, params...)
}
