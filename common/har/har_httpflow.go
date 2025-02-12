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

	// get post data
	contentType := lowhttp.GetHTTPPacketContentType(reqByte)
	var params []*HARHTTPParam
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
		Text:     string(lowhttp.GetHTTPPacketBody(reqByte)),
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// get content
	response.Content = &HARHTTPContent{
		Size:     len(body),
		MimeType: ResponseContentType,
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
