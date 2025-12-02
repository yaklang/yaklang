package har

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// HTTPFlow2HarEntryOptions 控制 HAR 导出的字段选择
type HTTPFlow2HarEntryOptions struct {
	// 选择的字段列表，如果为空则包含所有字段
	SelectedFields []string
}

// HTTPFlow2HarEntry 将 HTTPFlow 转换为 HAREntry
// 如果提供了 options，则根据字段选择来决定包含哪些内容
func HTTPFlow2HarEntry(flow *schema.HTTPFlow, options ...*HTTPFlow2HarEntryOptions) (*HAREntry, error) {
	var opts *HTTPFlow2HarEntryOptions
	if len(options) > 0 {
		opts = options[0]
	}
	
	// 检查字段是否被选中 - 支持多种字段名格式（前端可能传递不同的字段名）
	hasField := func(fieldNames ...string) bool {
		if opts == nil || len(opts.SelectedFields) == 0 {
			return true // 如果没有指定字段选择，默认包含所有字段
		}
		for _, fieldName := range fieldNames {
			for _, f := range opts.SelectedFields {
				// 支持精确匹配和忽略大小写匹配
				if strings.EqualFold(f, fieldName) {
					return true
				}
			}
		}
		return false
	}
	//---------------- build request
	request := &HARRequest{
		Method:      flow.Method,
		URL:         flow.Url,
		HeadersSize: -1,
	}
	reqRaw, err := strconv.Unquote(flow.Request)
	if err != nil {
		return nil, err
	}
	reqByte := []byte(reqRaw)

	// get http version
	_, _, request.HTTPVersion = lowhttp.GetHTTPPacketFirstLine(reqByte)
	isRequestHTTP2 := strings.Contains(strings.ToLower(request.HTTPVersion), "http/2")

	// get query string
	var requestQueryString []*HARKVPair
	for key, values := range lowhttp.GetFullHTTPRequestQueryParams(reqByte) {
		for _, value := range values {
			requestQueryString = append(requestQueryString, &HARKVPair{
				Name:  key,
				Value: value,
			})
		}
	}
	request.QueryString = requestQueryString

	var requestHeaders []*HARKVPair
	// http2 extra header
	if isRequestHTTP2 {
		requestHeaders = append(requestHeaders, &HARKVPair{
			Name:  ":authority",
			Value: lowhttp.GetHTTPPacketHeader(reqByte, "Host"),
		})
		requestHeaders = append(requestHeaders, &HARKVPair{
			Name:  ":method",
			Value: flow.Method,
		})
		urlIns := utils.ParseStringToUrl(flow.Url)
		requestHeaders = append(requestHeaders, &HARKVPair{
			Name:  ":path",
			Value: urlIns.Path,
		})
	}

	// get headers
	for key, values := range lowhttp.GetHTTPPacketHeadersFull(reqByte) {
		// skip host header in http2
		if isRequestHTTP2 && strings.ToLower(key) == "host" {
			continue
		}
		// in http2, all headers are lowercase
		if isRequestHTTP2 {
			key = strings.ToLower(key)
		}
		for _, value := range values {
			requestHeaders = append(requestHeaders, &HARKVPair{
				Name:  key,
				Value: value,
			})
		}
	}
	request.Headers = requestHeaders
	request.HeadersSize = len(requestHeaders)

	// get post data - 根据字段选择决定是否包含 body
	// 支持多种字段名：request, 请求包, Request, RequestPacket
	includeRequestBody := hasField("request", "请求包", "Request", "RequestPacket")
	contentType := lowhttp.GetHTTPPacketContentType(reqByte)
	var params []*HARHTTPParam
	var requestBodyText string
	if includeRequestBody {
		requestBodyText = string(lowhttp.GetHTTPPacketBody(reqByte))
		if contentType == "application/x-www-form-urlencoded" {
			for postKey, postValues := range lowhttp.GetFullHTTPRequestPostParams(reqByte) {
				for _, value := range postValues {
					params = append(params, &HARHTTPParam{
						Name:  postKey,
						Value: value,
					})
				}
			}
		}
	}
	if includeRequestBody {
		request.PostData = &HARHTTPPostData{
			MimeType: contentType,
			Params:   params,
			Text:     requestBodyText,
		}
	}
	request.BodySize = int(flow.BodyLength)

	//---------------- build response
	response := &HARResponse{
		StatusCode: int(flow.StatusCode),
		StatusText: http.StatusText(int(flow.StatusCode)),
	}

	var r io.Reader
	if flow.Response != "" {
		respRaw, err := strconv.Unquote(flow.Response)
		if err != nil {
			return nil, err
		}
		r = bytes.NewBufferString(respRaw)
	} else {
		f, err := os.Open(flow.TooLargeResponseHeaderFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		f2, err := os.Open(flow.TooLargeResponseBodyFile)
		if err != nil {
			return nil, err
		}
		defer f2.Close()

		r = io.MultiReader(f, f2)
	}

	resp, err := utils.ReadHTTPResponseFromBufioReader(r, nil)
	if err != nil {
		return nil, err
	}

	// get http version
	response.HTTPVersion = resp.Proto
	isResponseHTTP2 := strings.Contains(strings.ToLower(response.HTTPVersion), "http/2")

	// get headers
	var (
		responseHeaders     []*HARKVPair
		ResponseContentType string
	)
	for key, headers := range resp.Header {
		if isResponseHTTP2 {
			key = strings.ToLower(key)
		}
		if strings.ToLower(key) == "content-type" {
			ResponseContentType = headers[0]
		}
		for _, header := range headers {
			responseHeaders = append(responseHeaders, &HARKVPair{
				Name:  key,
				Value: header,
			})
		}
	}
	response.Headers = responseHeaders
	response.HeadersSize = len(responseHeaders)

	// 根据字段选择决定是否包含 response body
	// 支持多种字段名：response, 响应包, Response, ResponsePacket
	includeResponseBody := hasField("response", "响应包", "Response", "ResponsePacket")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	bodySize := len(body)

	// get content
	if includeResponseBody {
		response.Content = &HARHTTPContent{
			Size:     bodySize,
			MimeType: ResponseContentType,
			Text:     string(body),
		}
	} else {
		// 只包含大小和类型，不包含内容（符合 HAR 标准，但减少文件大小）
		response.Content = &HARHTTPContent{
			Size:     bodySize,
			MimeType: ResponseContentType,
			Text:     "", // 不包含 body 内容
		}
	}
	response.BodySize = bodySize

	// clear and save httpflow metadata - 根据字段选择决定是否包含某些元数据
	metadata := &HTTPFlowMetaData{
		NoFixContentLength: flow.NoFixContentLength,
		IsHTTPS:            flow.IsHTTPS,
	}
	
	// 根据字段选择包含相应的元数据
	// 支持多种字段名格式（前端可能传递不同的字段名）
	if hasField("path", "Path") {
		metadata.Path = flow.Path
	}
	if hasField("source_type", "SourceType", "来源") {
		metadata.SourceType = flow.SourceType
	}
	if hasField("duration", "Duration", "延迟", "延迟(ms)", "DurationMs") {
		metadata.Duration = flow.Duration
	}
	if hasField("get_params_total", "GetParamsTotal", "参数", "Params") {
		metadata.GetParamsTotal = flow.GetParamsTotal
		metadata.PostParamsTotal = flow.PostParamsTotal
		metadata.CookieParamsTotal = flow.CookieParamsTotal
	}
	if hasField("ip", "IP", "IPAddress", "iP_address", "ip_address") {
		metadata.IPAddress = flow.IPAddress
		metadata.IPInteger = flow.IPInteger
	}
	if hasField("tags", "Tags") {
		metadata.Tags = flow.Tags
	}
	if hasField("payload", "Payload") {
		metadata.Payload = flow.Payload
	}
	if hasField("is_websocket", "IsWebsocket", "Websocket", "IsWebSocket") {
		metadata.IsWebsocket = flow.IsWebsocket
	}
	if hasField("from_plugin", "FromPlugin", "相关插件") {
		metadata.FromPlugin = flow.FromPlugin
	}
	if hasField("process_name", "ProcessName") {
		metadata.ProcessName = flow.ProcessName
	}
	if hasField("upload_online", "UploadOnline") {
		metadata.UploadOnline = flow.UploadOnline
	}
	
	entry := &HAREntry{
		Request:         request,
		Response:        response,
		ServerIPAddress: flow.RemoteAddr,
		MetaData:        metadata,
	}

	return entry, nil
}

func HarEntry2HTTPFlow(entry *HAREntry) (*schema.HTTPFlow, error) {
	//---------------- build request
	req := entry.Request
	urlIns, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}

	reqPacket := lowhttp.BasicRequest()
	isRequestHTTP2 := strings.Contains(strings.ToLower(req.HTTPVersion), "http/2")

	// build request first line
	reqPacket = lowhttp.ReplaceHTTPPacketFirstLine(reqPacket, fmt.Sprintf("%s %s %s", req.Method, urlIns.RequestURI(), strings.ToUpper(req.HTTPVersion)))

	// build request headers
	ReqHeaders := make(map[string]string)
	lo.ForEach(req.Headers, func(kv *HARKVPair, _ int) {
		name, value := kv.Name, kv.Value
		if isRequestHTTP2 {
			if strings.HasPrefix(kv.Name, ":") {
				if strings.ToLower(kv.Name) == ":authority" {
					ReqHeaders["Host"] = kv.Value
				}
				return
			}
			name = http.CanonicalHeaderKey(name)
		}
		ReqHeaders[name] = value
	})
	reqPacket = lowhttp.ReplaceAllHTTPPacketHeaders(reqPacket, ReqHeaders)

	// build request query string
	params := lowhttp.NewQueryParams(lowhttp.WithDisableAutoEncode(true))
	for _, kv := range req.QueryString {
		params.Add(kv.Name, kv.Value)
	}
	reqPacket = lowhttp.ReplaceHTTPPacketQueryParamRaw(reqPacket, params.Encode())

	// build request post data
	postData := req.PostData
	if postData != nil {
		if postData.Text != "" {
			reqPacket = lowhttp.ReplaceHTTPPacketBody(reqPacket, []byte(postData.Text), false)
		} else if len(postData.Params) > 0 {
			postParams := lowhttp.NewQueryParams(lowhttp.WithDisableAutoEncode(true))
			for _, kv := range postData.Params {
				postParams.Add(kv.Name, kv.Value)
			}
			reqPacket = lowhttp.ReplaceHTTPPacketBody(reqPacket, []byte(postParams.Encode()), false)
		}
	}

	//---------------- build response
	resp := entry.Response

	respPacket := lowhttp.BasicResponse()

	// build response first line
	respPacket = lowhttp.ReplaceHTTPPacketFirstLine(respPacket, fmt.Sprintf("%s %d %s", strings.ToUpper(resp.HTTPVersion), resp.StatusCode, resp.StatusText))
	isResponseHTTP2 := strings.Contains(strings.ToLower(resp.HTTPVersion), "http/2")

	// build response headers
	RespHeaders := make(map[string]string)
	lo.ForEach(resp.Headers, func(kv *HARKVPair, _ int) {
		name, value := kv.Name, kv.Value
		if isResponseHTTP2 {
			name = http.CanonicalHeaderKey(name)
		}
		RespHeaders[name] = value
	})
	respPacket = lowhttp.ReplaceAllHTTPPacketHeaders(respPacket, RespHeaders)

	// build response content
	if resp.Content != nil && resp.Content.Text != "" {
		respPacket = lowhttp.ReplaceHTTPPacketBody(respPacket, []byte(resp.Content.Text), false)
	}

	metadata := entry.MetaData
	flow := &schema.HTTPFlow{
		Method:      req.Method,
		Url:         req.URL,
		StatusCode:  int64(resp.StatusCode),
		Request:     strconv.Quote(string(reqPacket)),
		Response:    strconv.Quote(string(respPacket)),
		RemoteAddr:  entry.ServerIPAddress,
		BodyLength:  int64(resp.Content.Size),
		ContentType: resp.Content.MimeType,
	}
	if metadata != nil {
		flow.NoFixContentLength = metadata.NoFixContentLength
		flow.IsHTTPS = metadata.IsHTTPS
		flow.Path = metadata.Path
		flow.SourceType = metadata.SourceType
		flow.Duration = metadata.Duration
		flow.GetParamsTotal = metadata.GetParamsTotal
		flow.PostParamsTotal = metadata.PostParamsTotal
		flow.CookieParamsTotal = metadata.CookieParamsTotal
		flow.IPAddress = metadata.IPAddress
		flow.IPInteger = metadata.IPInteger
		flow.Tags = metadata.Tags
		flow.Payload = metadata.Payload
		flow.IsWebsocket = metadata.IsWebsocket
		flow.FromPlugin = metadata.FromPlugin
		flow.ProcessName = metadata.ProcessName
		flow.UploadOnline = metadata.UploadOnline
	}
	return flow, nil
}
