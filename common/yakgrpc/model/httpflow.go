package model

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/domainextractor"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	GlobalHTTPFlowCache = utils.NewTTLCache[*ypb.HTTPFlow](10 * time.Minute)
	cacheMu             = new(sync.RWMutex)
)

func utf8safe(i string) string {
	return utils.EscapeInvalidUTF8Byte([]byte(i))
}

func SetHTTPFlowCacheGRPCModel(f *schema.HTTPFlow, full bool, m *ypb.HTTPFlow) {
	if f == nil {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	GlobalHTTPFlowCache.Set(f.CalcCacheHash(full), m)
}

func getCacheHTTPFlowGRPCModel(f *schema.HTTPFlow, full bool) *ypb.HTTPFlow {
	if f == nil {
		return nil
	}
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if v, ok := GlobalHTTPFlowCache.Get(f.CalcCacheHash(full)); ok {
		return v
	}
	return nil
}

func ToHTTPFlowGRPCModel(f *schema.HTTPFlow, full bool) (*ypb.HTTPFlow, error) {
	return toHTTPFlowGRPCModel(f, full)
}

func ToHTTPFlowGRPCModelFull(f *schema.HTTPFlow) (*ypb.HTTPFlow, error) {
	return toHTTPFlowGRPCModel(f, true)
}

func toHTTPFlowGRPCModel(f *schema.HTTPFlow, full bool) (*ypb.HTTPFlow, error) {
	if flow := getCacheHTTPFlowGRPCModel(f, full); flow != nil {
		return flow, nil
	}
	flow := &ypb.HTTPFlow{
		Id:                         uint64(f.ID),
		IsHTTPS:                    f.IsHTTPS,
		Url:                        utf8safe(f.Url),
		SourceType:                 f.SourceType,
		Path:                       utf8safe(f.Path),
		Method:                     utf8safe(f.Method),
		BodyLength:                 f.BodyLength,
		ContentType:                utf8safe(f.ContentType),
		StatusCode:                 f.StatusCode,
		GetParamsTotal:             int64(f.GetParamsTotal),
		PostParamsTotal:            int64(f.PostParamsTotal),
		UpdatedAt:                  f.UpdatedAt.Unix(),
		CreatedAt:                  f.CreatedAt.Unix(),
		HostPort:                   utf8safe(f.RemoteAddr),
		IPAddress:                  utf8safe(f.IPAddress),
		Tags:                       f.Tags,
		NoFixContentLength:         f.NoFixContentLength,
		IsWebsocket:                f.IsWebsocket,
		WebsocketHash:              f.WebsocketHash,
		IsTooLargeResponse:         f.IsTooLargeResponse,
		TooLargeResponseBodyFile:   f.TooLargeResponseBodyFile,
		TooLargeResponseHeaderFile: f.TooLargeResponseHeaderFile,
		Payloads: lo.Map(strings.Split(f.Payload, ","), func(i string, _ int) string {
			return utf8safe(i)
		}),
	}
	// 设置 title
	var (
		unquotedResponse string
		unquotedRequest  string
		err              error
	)
	if f.Response != "" {
		unquotedResponse, err = strconv.Unquote(f.Response)
		if err != nil {
			log.Errorf("unquoted response failed: %s", err)
			fmt.Println(f.Response)
			unquotedResponse = f.Response
		}
	}
	flow.HtmlTitle = utf8safe(strings.TrimSpace(utils.ExtractTitleFromHTMLTitle(unquotedResponse, "")))

	if f.UpdatedAt.IsZero() {
		flow.UpdatedAt = time.Now().Unix()
		flow.CreatedAt = time.Now().Unix()
	}
	if f.Hash == "" {
		flow.Hash = f.CalcHash()
	} else {
		flow.Hash = f.Hash
	}
	host, port, _ := utils.ParseStringToHostPort(flow.Url)
	flow.HostPort = utils.HostPort(host, port)

	flow.BodySizeVerbose = utils.ByteSize(uint64(flow.BodyLength))

	if f.Request != "" {
		unquotedRequest, err = strconv.Unquote(f.Request)
		if err != nil {
			unquotedRequest = f.Request
			log.Errorf("unquoted request failed: %s", err)
			fmt.Println(f.Request)
		}
	}

	flow.RequestLength = int64(len(unquotedRequest))
	flow.RequestSizeVerbose = utils.ByteSize(uint64(len(unquotedRequest)))

	requireRequest := full || !f.IsRequestOversize
	requireResponse := full || !f.IsResponseOversize
	isStandardRequest := !flow.NoFixContentLength

	haveRequest := lo.IsNotEmpty(unquotedRequest)
	var requestBody []byte
	if requireRequest {
		// return request:
		// 1. request is not required (!full)
		// 2. not a large request
		// 3. request is not oversize
		flow.Request = []byte(unquotedRequest)
		if isStandardRequest && haveRequest {
			// unfix request:
			var requestURI string
			_, requestBody = lowhttp.SplitHTTPPacket(flow.Request, func(method string, requestUri string, proto string) error {
				requestURI = requestUri
				return nil
			}, nil, func(line string) string {
				k, v := lowhttp.SplitHTTPHeader(line)
				flow.RequestHeader = append(flow.RequestHeader, &ypb.HTTPHeader{
					Header: utf8safe(utils.ParseStringToVisible(k)),
					Value:  utf8safe(utils.ParseStringToVisible(v)),
				})
				return line
			})
			if flow.Path == "" {
				flow.Path = utf8safe(requestURI)
			}

			fReq, _ := mutate.NewFuzzHTTPRequest(flow.Request, mutate.OptHTTPS(flow.IsHTTPS))
			const maxParams = 150
			var index int
			if fReq != nil {
				for _, r := range fReq.GetCommonParams() {
					if index >= maxParams {
						break
					}
					fParam := &ypb.FuzzableParam{
						Position:  r.PositionVerbose(),
						ParamName: utf8safe(utils.ParseStringToVisible(r.Name())),
						IsHTTPS:   flow.IsHTTPS,
					}

					if full {
						// 详情模式，这个很耗时。
						fParam = FuzzParamsToGRPCFuzzableParam(r, flow.IsHTTPS)
					}
					fParam.ParamName = utf8safe(fParam.ParamName)
					if r.IsGetParams() {
						flow.GetParams = append(flow.GetParams, fParam)
					}
					if r.IsPostParams() {
						flow.PostParams = append(flow.PostParams, fParam)
					}
					if r.IsCookieParams() {
						flow.CookieParams = append(flow.CookieParams, fParam)
					}
					index++
				}

				flow.GetParamsTotal = int64(len(flow.GetParams))
				flow.PostParamsTotal = int64(len(flow.PostParams))
				flow.CookieParamsTotal = int64(len(flow.CookieParams))
			}
		}
	}

	haveResponse := lo.IsNotEmpty(unquotedResponse)
	var responseBody []byte
	if requireResponse {
		flow.Response = []byte(unquotedResponse)
		flow.DisableRenderStyles = len(flow.Response) > 2*1000*1000
		if isStandardRequest && haveResponse {
			_, responseBody = lowhttp.SplitHTTPPacket(flow.Response, nil, nil, func(line string) string {
				k, v := lowhttp.SplitHTTPHeader(line)
				flow.ResponseHeader = append(flow.ResponseHeader, &ypb.HTTPHeader{
					Header: utf8safe(utils.ParseStringToVisible(k)),
					Value:  utf8safe(utils.ParseStringToVisible(v)),
				})
				return line
			})
		}
	}

	// 这里用来标记一下，UTF8 支持情况，要根据情况提供给用户合理 body 建议处理方案
	if requireRequest {
		flow.InvalidForUTF8Request = !utf8.ValidString(unquotedRequest)
		if flow.InvalidForUTF8Request {
			flow.RawRequestBodyBase64 = codec.EncodeBase64(requestBody)
			// 这里需要修复请求，这是为了解决Content-Type与body中boundary不一致的问题
			fixReq := lowhttp.FixHTTPRequest(flow.Request)
			flow.SafeHTTPRequest = utils.EscapeInvalidUTF8Byte(lowhttp.ConvertHTTPRequestToFuzzTag(fixReq))
		}
	}

	if requireResponse {
		flow.InvalidForUTF8Response = !utf8.ValidString(unquotedResponse)
		if flow.InvalidForUTF8Response {
			flow.RawResponseBodyBase64 = codec.EncodeBase64(responseBody)
		}
	}

	// 提取数据 - 完整详细的数据的时候，才应该提取
	if full {
		domains, rootDomains := domainextractor.ExtractDomainsEx(string(flow.Response))
		var jsonObjects []string
		if !utils.MatchAnyOfSubString(strings.ToLower(f.ContentType), "json") {
			if len(flow.Response) > 200*1000 {
				jsonObjects = jsonextractor.ExtractStandardJSON(string(flow.Response[:200*1000]))
			} else {
				jsonObjects = jsonextractor.ExtractStandardJSON(string(flow.Response))
			}
		}
		flow.Domains = make([]string, len(domains))
		for i, d := range domains {
			flow.Domains[i] = utf8safe(d)
		}
		flow.RootDomains = make([]string, len(rootDomains))
		for i, d := range rootDomains {
			flow.RootDomains[i] = utf8safe(d)
		}
		for _, j := range jsonObjects {
			if j == "" || j == "&&" || j == "{}" {
				continue
			}

			if !utf8.ValidString(j) {
				continue
			}
			if strings.HasPrefix(j, `{"":`) {
				continue
			}
			flow.JsonObjects = append(flow.JsonObjects, utf8safe(j))
		}
	}
	SetHTTPFlowCacheGRPCModel(f, full, flow)
	return flow, nil
}

func FuzzParamsToGRPCFuzzableParam(r *mutate.FuzzHTTPRequestParam, isHttps bool) *ypb.FuzzableParam {
	p := &ypb.FuzzableParam{
		Position:  r.PositionVerbose(),
		ParamName: utf8safe(r.Name()),
		IsHTTPS:   isHttps,
	}

	switch ret := r.Value().(type) {
	case []string:
		if len(ret) == 1 {
			p.OriginValue = []byte(ret[0])
		} else {
			p.OriginValue = utils.InterfaceToBytes(ret)
		}
	default:
		p.OriginValue = utils.InterfaceToBytes(ret)
	}

	flag := utils.RandNumberStringBytes(6)
	res, err := r.FriendlyDisplay().Fuzz(flag).Results()
	if err != nil {
		return p
	}

	var raw []byte
	for _, r := range res {
		raw, err = utils.HttpDumpWithBody(r, true)
		if err != nil {
			continue
		}
	}
	if raw != nil {
		if bytes.Contains(raw, []byte(flag)) {
			p.AutoTemplate = bytes.ReplaceAll(raw, []byte(flag), []byte("{{randstr(10,10,1)}}"))
		} else if bytes.Contains(raw, []byte(codec.EncodeBase64(flag))) {
			p.AutoTemplate = bytes.ReplaceAll(raw, []byte(codec.EncodeBase64(flag)), []byte("{{base64({{randstr(10,10,1)}})}}"))
		}
	}
	return p
}
