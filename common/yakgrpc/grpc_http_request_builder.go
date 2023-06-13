package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v3"
	"strings"
)

//go:embed grpc_http_request_builder_scripts.yak
var debugScript string

func (s *Server) DebugPlugin(req *ypb.DebugPluginRequest, stream ypb.Yak_DebugPluginServer) error {
	if req.GetInput() == "" {
		return utils.Error("input is empty")
	}

	if req.GetCode() == "" {
		return utils.Error("code is empty")
	}

	switch strings.ToLower(req.GetPluginType()) {
	case "mitm":
		fallthrough
	case "port-scan":
		fallthrough
	case "nuclei":
	default:
		return utils.Error("unsupported plugin type: " + req.GetPluginType())
	}

	var builderResponse, _ = s.HTTPRequestBuilder(stream.Context(), req.GetHTTPRequestTemplate())

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
		for _, res := range utils.PrettifyListFromStringSplitEx(req.GetInput(), "\n", "|", ",") {
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
			for _, res := range utils.PrettifyListFromStringSplitEx(req.GetInput(), "\n", "|", ",") {
				host, port, _ := utils.ParseStringToHostPort(res)
				if host == "" {
					host = res
				}
				if host == "" {
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

					feed(bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(utils.HostPort(host, port))), i.GetIsHttps())
				} else {
					feed(bytes.ReplaceAll(i.HTTPRequest, []byte(`{{Hostname}}`), []byte(host)), i.GetIsHttps())
				}
			}
		})
	}
	if len(reqs) <= 0 {
		return utils.Error("build http request failed: no results")
	}

	tempName := fmt.Sprintf("tmp-%v", ksuid.New().String())
	err := yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), tempName, &yakit.YakScript{
		ScriptName: tempName,
		Type:       req.GetPluginType(),
		Content:    req.GetCode(),
		Ignored:    false,
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := recover(); err != nil {
			log.Warn(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}

		log.Infof("start to remove temp plugin: %v", tempName)
		err = yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), tempName)
		if err != nil {
			log.Errorf("remove temp plugin failed: %v", err)
		}
	}()

	// 不同的插件类型，需要不同的处理
	switch strings.ToLower(req.GetPluginType()) {
	case "mitm":
	case "nuclei":
	case "port-scan":
	default:
		return utils.Error("unsupported plugin type: " + req.GetPluginType())
	}

	var feedbackClient = yaklib.NewVirtualYakitClient(func(i interface{}) error {
		switch ret := i.(type) {
		case *ypb.ExecResult:
			stream.Send(ret)
		case *yaklib.YakitLog:
			stream.Send(yaklib.NewYakitLogExecResult(ret.Level, ret.Data))
		default:
			spew.Dump(i)
		}
		return nil
	})
	engine := yak.NewScriptEngine(10)
	subEngine, err := engine.ExecuteExWithContext(stream.Context(), debugScript, map[string]any{
		"REQUESTS":     reqs,
		"CTX":          stream.Context(),
		"PLUGIN_NAME":  tempName,
		"YAKIT_CLIENT": feedbackClient,
	})
	if err != nil {
		log.Warnf("execute debug script failed: %v", err)
		return err
	}
	_ = subEngine

	return nil
}

func (s *Server) HTTPRequestBuilder(ctx context.Context, req *ypb.HTTPRequestBuilderParams) (*ypb.HTTPRequestBuilderResponse, error) {
	var isHttps = req.GetIsHttps()
	const tempTag = "[[__REPLACE_ME__]]"

	if req.GetIsRawHTTPRequest() {
		var reqStr = string(req.GetRawHTTPRequest())

		freq, err := mutate.NewFuzzHTTPRequest(reqStr)
		if err != nil {
			return nil, err
		}

		results, err := freq.FuzzHTTPHeader("Host", tempTag).Results()
		if err != nil {
			return nil, err
		}
		var firstReqStr string
		var handledRequest [][]byte
		for _, result := range results {
			raw, err := utils.HttpDumpWithBody(result, true)
			if err != nil {
				continue
			}
			raw = bytes.ReplaceAll(raw, []byte(tempTag), []byte("{{Hostname}}"))
			raw = bytes.ReplaceAll(raw, []byte(lowhttp.CRLF), []byte{'\n'})
			if firstReqStr == "" {
				firstReqStr = string(raw)
			}
			handledRequest = append(handledRequest, raw)
		}

		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		err = encoder.Encode(map[string]any{
			"requests": map[string]any{
				"raw": []string{
					firstReqStr,
				},
			},
		})
		if err != nil {
			return nil, err
		}

		var reqs []*ypb.HTTPRequestBuilderResult
		for _, r := range handledRequest {
			reqs = append(reqs, &ypb.HTTPRequestBuilderResult{
				IsHttps:     isHttps,
				HTTPRequest: r,
			})
		}
		templates := utils.EscapeInvalidUTF8Byte(buf.Bytes())
		return &ypb.HTTPRequestBuilderResponse{
			Results:   reqs,
			Templates: templates,
		}, nil
	}
	_ = isHttps

	var freq mutate.FuzzHTTPRequestIf = mutate.NewMustFuzzHTTPRequest(`GET / HTTP/1.1
Host: example.com
`)
	freq = freq.FuzzHTTPHeader("Host", tempTag)
	if req.GetMethod() == "" {
		freq = freq.FuzzMethod("GET")
	} else {
		freq = freq.FuzzMethod(req.GetMethod())
	}

	var paths []string
	var headers map[string]string
	if len(req.GetPath()) > 0 {
		freq = freq.FuzzPath(req.GetPath()...)
	}
	for _, p := range req.GetGetParams() {
		freq = freq.FuzzGetParams(p.GetKey(), p.GetValue())
	}
	for _, p := range req.GetCookie() {
		freq = freq.FuzzCookie(p.GetKey(), p.GetValue())
	}
	for _, p := range req.GetHeaders() {
		freq = freq.FuzzCookie(p.GetKey(), p.GetValue())
	}
	for _, p := range req.GetPostParams() {
		freq = freq.FuzzPostParams(p.GetKey(), p.GetValue())
	}
	if len(req.GetBody()) > 0 {
		freq = freq.FuzzPostRaw(string(req.GetBody()))
	}

	var method = ""
	var body string
	var results []*ypb.HTTPRequestBuilderResult
	if res, _ := freq.Results(); len(res) > 0 {
		for _, r := range res {
			raw, _ := utils.HttpDumpWithBody(r, true)
			if raw == nil || len(raw) <= 0 {
				continue
			}
			raw = bytes.ReplaceAll(raw, []byte(tempTag), []byte("{{Hostname}}"))

			results = append(results, &ypb.HTTPRequestBuilderResult{
				IsHttps:     isHttps,
				HTTPRequest: raw,
			})

			paths = append(paths, "{{BaseURL}}"+r.RequestURI)
			if method == "" {
				method = r.Method
			}
			if body == "" {
				_, bodyRaw := lowhttp.SplitHTTPHeadersAndBodyFromPacket(raw)
				body = string(bodyRaw)
			}
			for k, v := range r.Header {
				switch strings.ToLower(k) {
				case "host":
					continue
				}
				if len(v) <= 0 {
					continue
				}
				if headers == nil {
					headers = make(map[string]string)
				}
				headers[k] = v[0]
			}

		}
	}
	if len(paths) <= 0 {
		return nil, utils.Errorf("no path found")
	}

	var reqIns = map[string]any{
		"method": method,
		"path":   paths,
	}
	if headers != nil && len(headers) > 0 {
		reqIns["headers"] = headers
	}
	if body != "" {
		reqIns["body"] = body
	}
	var data = map[string]any{
		"requests": []any{reqIns},
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	encoder.Encode(&data)
	encoder.Close()
	templates := utils.EscapeInvalidUTF8Byte(buf.Bytes())
	return &ypb.HTTPRequestBuilderResponse{Templates: templates, Results: results}, nil
}
