package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"net/url"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"google.golang.org/grpc"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v3"
)

//go:embed grpc_http_request_builder_scripts.yak
var debugScriptCode string

func (s *Server) DebugPlugin(req *ypb.DebugPluginRequest, stream ypb.Yak_DebugPluginServer) error {
	input := req.GetInput()
	pluginType := req.GetPluginType()
	if !lo.Contains([]string{"yak", "codec", "mitm", "nuclei", "port-scan"}, pluginType) {
		return utils.Errorf("unsupported plugin type: %#v", pluginType)
	}

	if pluginType != "yak" && input == "" && req.GetHTTPRequestTemplate() == nil {
		return utils.Error("input / input packet is empty")
	}

	invalidRequest := pluginType != "yak" && input == "" && !req.GetHTTPRequestTemplate().GetIsRawHTTPRequest() // 非 yak 插件没有 input 也没有 http request
	if invalidRequest {
		return utils.Error("cannot find/extract debug target")
	}

	execParams := req.GetExecParams()
	if pluginType == "yak" && req.GetLinkPluginConfig() != nil { // yak 类型插件 构造联动插件参数
		LinkPluginList := s.PluginListGenerator(req.GetLinkPluginConfig(), stream.Context())
		replace := false
		for i := 0; i < len(execParams); i++ {
			if execParams[i].GetKey() == "__yakit_plugin_names__" {
				execParams[i].Value = strings.Join(LinkPluginList, "|")
				replace = true
			}
		}
		if !replace {
			execParams = append(execParams, &ypb.KVPair{Key: "__yakit_plugin_names__", Value: strings.Join(LinkPluginList, "|")})
		}
	}

	// 传递 RenderFuzzTag 参数给调试脚本
	if req.GetRenderFuzzTag() {
		execParams = append(execParams, &ypb.KVPair{Key: "FuzzTag", Value: "true"})
	}

	runtimeId := req.GetRuntimeId()
	if runtimeId == "" {
		runtimeId = uuid.New().String()
	}

	if req.GetPluginName() != "" {
		return s.execScript(input, req.GetPluginType(), req.GetPluginName(), stream, execParams, runtimeId, req.GetHTTPRequestTemplate())
	}
	return s.debugScript(input, req.GetPluginType(), req.GetCode(), stream, execParams, runtimeId, req.GetHTTPRequestTemplate())
}

func (s *Server) HTTPRequestBuilder(ctx context.Context, req *ypb.HTTPRequestBuilderParams) (*ypb.HTTPRequestBuilderResponse, error) {
	return httpRequestBuilder(ctx, req)
}

func httpRequestBuilder(ctx context.Context, req *ypb.HTTPRequestBuilderParams) (*ypb.HTTPRequestBuilderResponse, error) {
	isHttps := req.GetIsHttps()
	const tempTag = "[[__REPLACE_ME__]]"

	if req.GetIsRawHTTPRequest() {
		reqStr := string(req.GetRawHTTPRequest())

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
		freq = freq.FuzzHTTPHeader(p.GetKey(), p.GetValue())
	}
	for _, p := range req.GetPostParams() {
		freq = freq.FuzzPostParams(p.GetKey(), p.GetValue())
	}
	for _, p := range req.GetMultipartParams() {
		freq = freq.FuzzUploadKVPair(p.GetKey(), p.GetValue())
	}
	for _, p := range req.GetMultipartFileParams() {
		freq = freq.FuzzUploadFileName(p.GetKey(), p.GetValue())
	}
	if len(req.GetBody()) > 0 {
		freq = freq.FuzzPostRaw(string(req.GetBody()))
	}

	method := ""
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

	reqIns := map[string]any{
		"method": method,
		"path":   paths,
	}
	if headers != nil && len(headers) > 0 {
		reqIns["headers"] = headers
	}
	if body != "" {
		reqIns["body"] = body
	}
	data := map[string]any{
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

func (s *Server) PluginListGenerator(plugin *ypb.HybridScanPluginConfig, ctx context.Context) (res []string) {
	// 生成插件列表参数
	for _, i := range plugin.GetPluginNames() {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase().Model(&schema.YakScript{}), i)
		if err != nil {
			continue
		}
		res = append(res, script.ScriptName)
	}
	if plugin.GetFilter() != nil {
		for pluginInstance := range yakit.YieldYakScripts(yakit.FilterYakScript(
			s.GetProfileDatabase().Model(&schema.YakScript{}), plugin.GetFilter(),
		), ctx) {
			res = append(res, pluginInstance.ScriptName)
		}
	}
	return
}

type HTTPRequestBuilderRes struct {
	IsHttps bool
	Request []byte
	Url     string
}

func BuildHttpRequestPacket(db *gorm.DB, baseBuilderParams *ypb.HTTPRequestBuilderParams, targetInput string) (chan *HTTPRequestBuilderRes, error) {
	builderRes := make(chan *HTTPRequestBuilderRes)
	if baseBuilderParams != nil {
		if baseBuilderParams.GetIsRawHTTPRequest() {
			reqUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(baseBuilderParams.RawHTTPRequest, baseBuilderParams.IsHttps)
			if err != nil {
				return nil, err
			}
			go func() {
				defer close(builderRes)
				builderRes <- &HTTPRequestBuilderRes{
					IsHttps: baseBuilderParams.IsHttps,
					Request: baseBuilderParams.RawHTTPRequest,
					Url:     reqUrl.String(),
				}
			}()
			return builderRes, nil
		}

		if baseBuilderParams.GetIsHttpFlowId() {
			_, flows, err := yakit.QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
				IncludeId: baseBuilderParams.GetHTTPFlowId(),
			})
			if err != nil {
				return nil, err
			}
			go func() {
				defer close(builderRes)
				for _, flow := range flows {
					builderRes <- &HTTPRequestBuilderRes{
						IsHttps: flow.IsHTTPS,
						Request: codec.StrConvUnquoteForce(flow.Request),
						Url:     flow.Url,
					}
				}
			}()
			return builderRes, nil
		}
	}

	targets := make(chan *url.URL)
	go func() {
		defer close(targets)
		for _, target := range utils.PrettifyListFromStringSplitEx(targetInput, "\n", "|", ",") {
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			if utils.IsValidHost(target) { // 处理没有单独一个host情况 不含port
				targets <- &url.URL{Host: target, Path: "/"}
				continue
			}
			urlIns := utils.ParseStringToUrl(target)
			if urlIns.Host == "" {
				continue
			}

			host, port, _ := utils.ParseStringToHostPort(urlIns.Host) // 处理包含 port 的情况
			if !utils.IsValidHost(host) {                             // host不合规情况 比如 a:80
				continue
			}

			if port > 0 && urlIns.Scheme == "" { // fix https
				if port == 443 {
					urlIns.Scheme = "https"
				}
			}
			if urlIns.Path == "" {
				urlIns.Path = "/"
			}
			targets <- urlIns
		}
	}()

	go func() {
		defer close(builderRes)
		baseTemplates := []byte("GET {{Path}} HTTP/1.1\r\nHost: {{Hostname}}\r\n\r\n")

		for target := range targets {
			builderParams := mergeBuildParams(baseBuilderParams, target)
			if builderParams == nil {
				continue
			}
			builderResponse, err := httpRequestBuilder(context.Background(), builderParams)
			if err != nil {
				log.Errorf("failed to build http request: %v", err)
			}
			results := builderResponse.GetResults()
			if len(results) <= 0 {
				packet := bytes.ReplaceAll(baseTemplates, []byte(`{{Hostname}}`), []byte(target.Host))
				packet = bytes.ReplaceAll(packet, []byte(`{{Path}}`), []byte(target.Path))
				builderRes <- &HTTPRequestBuilderRes{IsHttps: target.Scheme == "https", Request: packet, Url: target.String()}
			} else {
				for _, result := range results {
					packet := bytes.ReplaceAll(result.HTTPRequest, []byte(`{{Hostname}}`), []byte(target.Host))
					tUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(packet, result.IsHttps)
					if err != nil || tUrl == nil {
						log.Errorf("failed to extract url from request: %v for error: %v", packet, err)
						continue
					}
					builderRes <- &HTTPRequestBuilderRes{IsHttps: result.IsHttps, Request: packet, Url: tUrl.String()}
				}
			}
		}
	}()
	return builderRes, nil
}

type wrapperDebugPluginStream struct {
	root      ypb.Yak_DebugPluginServer
	ctx       context.Context
	sendMutex *sync.Mutex
	sendHook  func(r *ypb.ExecResult) *ypb.ExecResult
	grpc.ServerStream
}

func newWrapperDebugPluginStream(ctx context.Context, stream ypb.Yak_DebugPluginServer) *wrapperDebugPluginStream {
	return &wrapperDebugPluginStream{
		root: stream, ctx: ctx,
		sendMutex: new(sync.Mutex),
	}
}

func (w *wrapperDebugPluginStream) Send(r *ypb.ExecResult) error {
	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()
	return w.root.Send(w.sendHook(r))
}

func (w *wrapperDebugPluginStream) registerSendHook(hookFunc func(r *ypb.ExecResult) *ypb.ExecResult) {
	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()
	w.sendHook = hookFunc
}

func (s *Server) CheckHahValidAiConfig(context.Context, *ypb.Empty) (*ypb.GeneralResponse, error) {
	tmps, err := s.GetThirdPartyAppConfigTemplate(context.Background(), &ypb.Empty{})
	if err != nil {
		return nil, err
	}
	unnecessaryApiKeyAndOk := func(name string) bool {
		for _, item := range tmps.GetTemplates() {
			if item.GetName() == name {
				for _, item := range item.Items {
					if item.GetName() == "api_key" {
						return item.Required
					}
				}
			}
		}
		return false
	}
	for _, name := range aispec.RegisteredAIGateways() {
		cfg := &aispec.AIConfig{}
		consts.GetThirdPartyApplicationConfig(name, cfg)
		if cfg.APIKey != "" || unnecessaryApiKeyAndOk(name) {
			return &ypb.GeneralResponse{Ok: true}, nil
		}
	}
	return &ypb.GeneralResponse{Ok: false}, nil
}
