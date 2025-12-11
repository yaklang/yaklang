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

	// 检查字段是否被选中 - 自动支持忽略大小写匹配，也支持多个别名（如中文别名）
	// 不传递FieldName时，不包含任何字段（返回false）
	hasField := func(fieldNames ...string) bool {
		if opts == nil || len(opts.SelectedFields) == 0 {
			return false // 如果没有指定字段选择，不包含任何字段
		}
		for _, fieldName := range fieldNames {
			for _, f := range opts.SelectedFields {
				// 自动支持忽略大小写匹配（对于英文字段名）
				// 也支持精确匹配（对于中文别名等）
				if strings.EqualFold(f, fieldName) {
					return true
				}
			}
		}
		return false
	}

	// 检查是否传递了父级字段（如"request"或"response"）
	hasParentField := func(parentFieldNames ...string) bool {
		return hasField(parentFieldNames...)
	}
	//---------------- build request
	// HAR格式要求request对象必须存在，但只包含用户勾选的字段
	// 检查是否传递了父级字段"request"（如果传递了，包含所有request子字段）
	hasRequestParent := hasParentField("request")
	// 检查是否传递了request相关的子字段
	// body_length 是响应大小，不应该在这里处理
	hasRequestFields := hasField("method", "url", "request", "headers", "request_length", "host")

	// 总是创建request对象（HAR格式要求），但只包含用户勾选的字段
	// 不设置任何默认值，只包含用户勾选的字段
	var request *HARRequest

	// 如果传递了request相关字段，解析和填充数据
	if hasRequestFields {
		request = &HARRequest{}

		// 如果传递了父级字段"request"，包含所有子字段；否则只包含传递的子字段
		includeMethod := hasRequestParent || hasField("method")
		includeURL := hasRequestParent || hasField("url")
		includeHeaders := hasRequestParent || hasField("headers")
		includeBody := hasRequestParent || hasField("request")

		// 只有勾选了method才包含（可以直接从flow.Method获取，不需要解析）
		if includeMethod {
			request.Method = flow.Method
		}

		// 只有勾选了url才包含（可以直接从flow.Url获取，不需要解析）
		if includeURL {
			request.URL = flow.Url
		}

		// 只有当需要解析headers、queryString或body时，才解析reqByte
		needParseRequestPacket := includeHeaders || includeBody || (includeURL && hasRequestParent)
		var reqByte []byte
		if needParseRequestPacket {
			reqRaw, err := strconv.Unquote(flow.Request)
			if err != nil {
				return nil, err
			}
			reqByte = []byte(reqRaw)

			// 如果需要解析HTTP版本（需要headers或body或父级字段）
			if includeHeaders || includeBody || hasRequestParent {
				// get http version
				_, _, request.HTTPVersion = lowhttp.GetHTTPPacketFirstLine(reqByte)
				isRequestHTTP2 := strings.Contains(strings.ToLower(request.HTTPVersion), "http/2")

				// get query string - 如果勾选了url或父级字段request
				if includeURL || hasRequestParent {
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
				}

				// get headers - 如果勾选了headers或父级字段request
				if includeHeaders {
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
					// 只有勾选了headers时才设置HeadersSize
					request.HeadersSize = len(requestHeaders)
				}

				// get post data - 如果勾选了request字段或父级字段request，就解析并包含 body
				if includeBody {
					contentType := lowhttp.GetHTTPPacketContentType(reqByte)
					var params []*HARHTTPParam
					requestBodyText := string(lowhttp.GetHTTPPacketBody(reqByte))
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
					request.PostData = &HARHTTPPostData{
						MimeType: contentType,
						Params:   params,
						Text:     requestBodyText,
					}
				}
			}
		}

		// 只有勾选了request_length或父级字段request时才设置请求BodySize
		// body_length 是响应大小，不应该在这里处理
		includeRequestBodySize := hasRequestParent || hasField("request_length")
		if includeRequestBodySize {
			// 如果已经解析了请求体，使用实际解析的长度；否则使用 flow.RequestLength
			if includeBody && request.PostData != nil {
				request.BodySize = len(request.PostData.Text)
			} else {
				request.BodySize = int(flow.RequestLength)
			}
		}
	}

	//---------------- build response
	// HAR格式要求response对象必须存在，但只包含用户勾选的字段
	// 检查是否传递了父级字段"response"（如果传递了，包含所有response子字段）
	hasResponseParent := hasParentField("response")
	// 检查是否传递了response相关的子字段
	hasResponseFields := hasField("status_code", "response", "headers", "body_length", "content_type")

	// 总是创建response对象（HAR格式要求），但只包含用户勾选的字段
	// 不设置任何默认值，只包含用户勾选的字段
	var response *HARResponse
	// 用于存储从响应header提取的content-type，供metadata使用
	var ResponseContentType string

	// 如果传递了response相关字段，解析和填充数据
	if hasResponseFields {
		response = &HARResponse{}
		// 如果传递了父级字段"response"，包含所有子字段；否则只包含传递的子字段
		includeStatusCode := hasResponseParent || hasField("status_code")
		includeHeaders := hasResponseParent || hasField("headers")
		includeBody := hasResponseParent || hasField("response")

		// 只有勾选了status_code才包含
		if includeStatusCode {
			response.StatusCode = int(flow.StatusCode)
			response.StatusText = http.StatusText(int(flow.StatusCode))
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

		// 获取content-type（在解析body之前）
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			ResponseContentType = ct
		}

		// 如果需要解析HTTP版本和headers（需要status_code或父级字段）
		if includeStatusCode {
			// get http version
			response.HTTPVersion = resp.Proto
			isResponseHTTP2 := strings.Contains(strings.ToLower(response.HTTPVersion), "http/2")

			// get headers - 如果勾选了headers或父级字段response
			if includeHeaders {
				var responseHeaders []*HARKVPair
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
			}
		}

		// 检查是否勾选了body_length字段（响应大小）
		// request_length 是请求大小，不应该在这里处理
		includeBodySize := hasResponseParent || hasField("body_length")

		// 如果勾选了response字段或父级字段response，就解析并包含 body
		if includeBody {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			bodySize := len(body)

			response.Content = &HARHTTPContent{
				Size:     bodySize,
				MimeType: ResponseContentType,
				Text:     string(body),
			}
			// 只有勾选了bodySize或父级字段response时才设置BodySize
			if includeBodySize {
				response.BodySize = bodySize
			}
		} else if includeBodySize {
			// 如果只勾选了bodySize，需要读取body以获取大小，但不包含内容
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			bodySize := len(body)
			response.BodySize = bodySize
			// 不创建Content对象，因为只勾选了bodySize
		}
	} else if hasField("body_length") {
		// 如果只勾选了body_length但没有勾选其他response字段，使用flow.BodyLength
		response = &HARResponse{}
		response.BodySize = int(flow.BodyLength)
	}
	// 如果没有勾选response相关字段，不创建任何response子对象（保持为空）

	// clear and save httpflow metadata - 根据字段选择决定是否包含某些元数据
	// 检查是否有任何metadata字段被勾选
	hasMetadataFields := hasField("id", "path", "host", "source_type", "duration",
		"get_params_total", "ip", "ip_address", "iP_address", "tags", "payload", "payloads",
		"content_type", "is_websocket", "from_plugin", "process_name", "upload_online", "updated_at")

	var metadata *HTTPFlowMetaData
	if hasMetadataFields {
		metadata = &HTTPFlowMetaData{
			NoFixContentLength: flow.NoFixContentLength,
			IsHTTPS:            flow.IsHTTPS,
		}

		// 根据字段选择包含相应的元数据
		if hasField("id") {
			metadata.ID = flow.ID
		}
		if hasField("path") {
			metadata.Path = flow.Path
		}
		if hasField("host") {
			metadata.Host = flow.Host
		}
		if hasField("source_type") {
			metadata.SourceType = flow.SourceType
		}
		if hasField("duration") {
			metadata.Duration = flow.Duration
		}
		if hasField("get_params_total") {
			metadata.GetParamsTotal = flow.GetParamsTotal
			metadata.PostParamsTotal = flow.PostParamsTotal
			metadata.CookieParamsTotal = flow.CookieParamsTotal
		}
		if hasField("ip", "ip_address", "iP_address") {
			metadata.IPAddress = flow.IPAddress
			metadata.IPInteger = flow.IPInteger
		}
		if hasField("tags") {
			metadata.Tags = flow.Tags
		}
		if hasField("payload", "payloads") {
			metadata.Payload = flow.Payload
		}
		if hasField("content_type") {
			// 优先使用从响应header提取的content-type，如果没有则使用flow.ContentType
			if ResponseContentType != "" {
				metadata.ContentType = ResponseContentType
			} else {
				metadata.ContentType = flow.ContentType
			}
		}
		if hasField("is_websocket") {
			metadata.IsWebsocket = flow.IsWebsocket
		}
		if hasField("from_plugin") {
			metadata.FromPlugin = flow.FromPlugin
		}
		if hasField("process_name") {
			metadata.ProcessName = flow.ProcessName
		}
		if hasField("upload_online") {
			metadata.UploadOnline = flow.UploadOnline
		}
		if hasField("updated_at") {
			metadata.UpdatedAt = flow.UpdatedAt
		}
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
