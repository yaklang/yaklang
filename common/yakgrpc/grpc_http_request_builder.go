package yakgrpc

import (
	"bytes"
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v3"
	"strings"
)

//go:embed grpc_http_request_builder_scripts.yak
var debugScript string

func (s *Server) DebugPlugin(req *ypb.DebugPluginRequest, stream ypb.Yak_DebugPluginServer) error {
	return s.debugScript(req.GetInput(), req.GetPluginType(), req.GetCode(), stream, req.GetHTTPRequestTemplate())
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
