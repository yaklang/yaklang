package lowhttp

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type HTTPArchive struct {
	Entries []*HAREntry `json:"entries"`
}

type HAREntry struct {
	Request         *HARRequest  `json:"request"`
	Response        *HARResponse `json:"response"`
	ServerIPAddress string       `json:"serverIPAddress"`
}

type HARKVPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type HARHTTPParam struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
}

type HARHTTPPostData struct {
	MimeType string          `json:"mimeType"`
	Params   []*HARHTTPParam `json:"params"`
	Text     string          `json:"text"`
}

type HARRequest struct {
	Method      string           `json:"method"`
	URL         string           `json:"url"`
	HTTPVersion string           `json:"httpVersion"`
	QueryString []*HARKVPair     `json:"queryString"`
	Headers     []*HARKVPair     `json:"headers"`
	HeadersSize int              `json:"headersSize"`
	BodySize    int              `json:"bodySize"`
	PostData    *HARHTTPPostData `json:"postData"`
}

type HARHTTPContent struct {
	Size     int    `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
	Encoding string `json:"encoding"`
}

type HARResponse struct {
	StatusCode  int             `json:"status"`
	StatusText  string          `json:"statusText"`
	HTTPVersion string          `json:"httpVersion"`
	Headers     []*HARKVPair    `json:"headers"`
	Cookies     []*HARKVPair    `json:"cookies"`
	Content     *HARHTTPContent `json:"content"`
	HeadersSize int             `json:"headersSize"`
	BodySize    int             `json:"bodySize"`
}

func HTTPFlow2HarEntry(flow *schema.HTTPFlow) (*HAREntry, error) {
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
	_, _, request.HTTPVersion = GetHTTPPacketFirstLine(reqByte)
	isRequestHTTP2 := strings.Contains(strings.ToLower(request.HTTPVersion), "http/2")

	// get query string
	var requestQueryString []*HARKVPair
	for key, values := range GetFullHTTPRequestQueryParams(reqByte) {
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
			Value: GetHTTPPacketHeader(reqByte, "Host"),
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
	for key, values := range GetHTTPPacketHeadersFull(reqByte) {
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

	// get post data
	contentType := GetHTTPPacketContentType(reqByte)
	var params []*HARHTTPParam
	if contentType == "application/x-www-form-urlencoded" {
		for postKey, postValues := range GetFullHTTPRequestPostParams(reqByte) {
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
		Text:     string(GetHTTPPacketBody(reqByte)),
	}
	request.BodySize = int(flow.BodyLength)

	//---------------- build response
	response := &HARResponse{
		StatusCode: int(flow.StatusCode),
		StatusText: http.StatusText(int(flow.StatusCode)),
	}

	respRaw, err := strconv.Unquote(flow.Response)
	if err != nil {
		return nil, err
	}
	respByte := []byte(respRaw)

	// get http version
	response.HTTPVersion, _, _ = GetHTTPPacketFirstLine(respByte)
	isResponseHTTP2 := strings.Contains(strings.ToLower(response.HTTPVersion), "http/2")

	// get headers
	var responseHeaders []*HARKVPair
	for key, values := range GetHTTPPacketHeadersFull(respByte) {
		if isResponseHTTP2 {
			key = strings.ToLower(key)
		}
		for _, value := range values {
			responseHeaders = append(responseHeaders, &HARKVPair{
				Name:  key,
				Value: value,
			})
		}
	}
	response.Headers = responseHeaders
	response.HeadersSize = len(responseHeaders)

	body := GetHTTPPacketBody(respByte)

	// get content
	response.Content = &HARHTTPContent{
		Size:     len(body),
		MimeType: GetHTTPPacketContentType(respByte),
		Text:     string(body),
	}
	response.BodySize = len(body)

	entry := &HAREntry{
		Request:         request,
		Response:        response,
		ServerIPAddress: flow.RemoteAddr,
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

	reqPacket := BasicRequest()
	isRequestHTTP2 := strings.Contains(strings.ToLower(req.HTTPVersion), "http/2")

	// build request first line
	reqPacket = ReplaceHTTPPacketFirstLine(reqPacket, fmt.Sprintf("%s %s %s", req.Method, urlIns.RequestURI(), strings.ToUpper(req.HTTPVersion)))

	// build request headers
	var ReqHeaders map[string]string
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
	reqPacket = ReplaceAllHTTPPacketHeaders(reqPacket, ReqHeaders)

	// build request query string
	params := NewQueryParams(WithDisableAutoEncode(true))
	for _, kv := range req.QueryString {
		params.Add(kv.Name, kv.Value)
	}
	reqPacket = ReplaceHTTPPacketQueryParamRaw(reqPacket, params.Encode())

	// build request post data
	postData := req.PostData
	if postData.Text != "" {
		reqPacket = ReplaceHTTPPacketBody(reqPacket, []byte(postData.Text), false)
	} else if len(postData.Params) > 0 {
		postParams := NewQueryParams(WithDisableAutoEncode(true))
		for _, kv := range postData.Params {
			postParams.Add(kv.Name, kv.Value)
		}
		reqPacket = ReplaceHTTPPacketBody(reqPacket, []byte(postParams.Encode()), false)
	}

	//---------------- build response
	resp := entry.Response

	respPacket := BasicResponse()

	// build response first line
	respPacket = ReplaceHTTPPacketFirstLine(respPacket, fmt.Sprintf("%s %d %s", strings.ToUpper(resp.HTTPVersion), resp.StatusCode, resp.StatusText))
	isResponseHTTP2 := strings.Contains(strings.ToLower(resp.HTTPVersion), "http/2")

	// build response headers
	var RespHeaders map[string]string
	lo.ForEach(resp.Headers, func(kv *HARKVPair, _ int) {
		name, value := kv.Name, kv.Value
		if isResponseHTTP2 {
			name = http.CanonicalHeaderKey(name)
		}
		RespHeaders[name] = value
	})
	reqPacket = ReplaceAllHTTPPacketHeaders(reqPacket, RespHeaders)

	// build response content
	if resp.Content.Text != "" {
		respPacket = ReplaceHTTPPacketBody(respPacket, []byte(resp.Content.Text), false)
	}

	return &schema.HTTPFlow{
		Method:      req.Method,
		Url:         req.URL,
		StatusCode:  int64(resp.StatusCode),
		Request:     strconv.Quote(string(reqPacket)),
		Response:    strconv.Quote(string(respPacket)),
		RemoteAddr:  entry.ServerIPAddress,
		BodyLength:  int64(resp.Content.Size),
		ContentType: resp.Content.MimeType,
		SourceType:  schema.HTTPFlow_SourceType_HAR,
	}, nil
}
