package har

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestHAR2HTTPFlow(t *testing.T) {
	wantRspBody := "Hello World"
	randTag := utils.RandStringBytes(16)
	entry := HAREntry{
		Request: &HARRequest{
			Method:      "GET",
			URL:         "https://example.com",
			HTTPVersion: "http/2.0",
			Headers: []*HARKVPair{
				{
					Name:  ":authority",
					Value: "example.com",
				},
				{
					Name:  ":method",
					Value: "GET",
				},
				{
					Name:  ":path",
					Value: "/",
				},
				{
					Name:  ":scheme",
					Value: "https",
				},
				{
					Name:  "user-agent",
					Value: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
				},
			},
			HeadersSize: -1,
			BodySize:    0,
		},
		Response: &HARResponse{
			StatusCode:  200,
			StatusText:  "OK",
			HTTPVersion: "http/2.0",
			HeadersSize: -1,
			Headers: []*HARKVPair{
				{
					Name:  "content-length",
					Value: strconv.Itoa(len(wantRspBody)),
				},
				{
					Name:  "content-type",
					Value: "text/html",
				},
				{
					Name:  "vary",
					Value: "Accept-Encoding",
				},
			},
			Content: &HARHTTPContent{
				Size:     len(wantRspBody),
				MimeType: "text/html",
				Text:     wantRspBody,
			},
			BodySize: -1,
		},
		MetaData: &HTTPFlowMetaData{
			SourceType: "har",
			Tags:       randTag,
		},
	}
	flow, err := HarEntry2HTTPFlow(&entry)
	require.NoError(t, err)
	require.Equal(t, "GET", flow.Method)
	require.Equal(t, int64(200), flow.StatusCode)
	require.Equal(t, "https://example.com", flow.Url)
	// check request
	req, err := strconv.Unquote(flow.Request)
	require.NoError(t, err)
	reqBytes := []byte(req)
	_, _, version := lowhttp.GetHTTPPacketFirstLine(reqBytes)
	require.Equal(t, "HTTP/2.0", version)
	require.Equal(t, "example.com", lowhttp.GetHTTPPacketHeader(reqBytes, "Host"))
	require.Equal(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36", lowhttp.GetHTTPPacketHeader(reqBytes, "User-Agent"))
	// check response
	rsp, err := strconv.Unquote(flow.Response)
	require.NoError(t, err)
	rspBytes := []byte(rsp)
	respVersion, statusCode, statusMessage := lowhttp.GetHTTPPacketFirstLine(rspBytes)
	require.Equal(t, "HTTP/2.0", respVersion)
	require.Equal(t, "200", statusCode)
	require.Equal(t, "OK", statusMessage)
	require.Equal(t, strconv.Itoa(len(wantRspBody)), lowhttp.GetHTTPPacketHeader(rspBytes, "Content-Length"))
	require.Equal(t, "text/html", lowhttp.GetHTTPPacketHeader(rspBytes, "Content-Type"))
	require.Equal(t, "Accept-Encoding", lowhttp.GetHTTPPacketHeader(rspBytes, "Vary"))
	require.Equal(t, wantRspBody, string(lowhttp.GetHTTPPacketBody(rspBytes)))
	// check metadata
	require.Equal(t, "har", flow.SourceType)
	require.Equal(t, randTag, flow.Tags)
}

func TestHTTPFlow2HAR(t *testing.T) {
	// generate a random data
	host := fmt.Sprintf("%s.com", utils.RandStringBytes(16))
	path := fmt.Sprintf("/%s", utils.RandStringBytes(16))
	url := fmt.Sprintf(`https://%s%s`, host, path)
	requestBody := utils.RandStringBytes(16)
	responseBody := fmt.Sprintf(`<html><body><p>%s</p><body></html>`, utils.RandStringBytes(16))
	queryParams := make([]*HARKVPair, 16)
	for i := 0; i < 16; i++ {
		queryParams[i] = &HARKVPair{
			Name:  utils.RandStringBytes(16),
			Value: utils.RandStringBytes(16),
		}
	}
	queryString := strings.Join(lo.Map(queryParams, func(item *HARKVPair, _ int) string {
		return fmt.Sprintf("%s=%s", item.Name, item.Value)
	}), "&")

	headers := make([]*HARKVPair, 16, 17)
	for i := 0; i < 16; i++ {
		headers[i] = &HARKVPair{
			Name:  utils.RandStringBytes(16),
			Value: utils.RandStringBytes(16),
		}
	}
	headerString := strings.Join(lo.Map(headers, func(item *HARKVPair, _ int) string {
		return fmt.Sprintf("%s: %s", item.Name, item.Value)
	}), "\n")
	responseHeaders := make([]*HARKVPair, 16, 17)
	for i := 0; i < 16; i++ {
		responseHeaders[i] = &HARKVPair{
			Name:  utils.RandStringBytes(16),
			Value: utils.RandStringBytes(16),
		}
	}
	responseHeaderString := strings.Join(lo.Map(responseHeaders, func(item *HARKVPair, _ int) string {
		return fmt.Sprintf("%s: %s", item.Name, item.Value)
	}), "\n")

	request := fmt.Sprintf(`POST %s?%s HTTP/2
%s
Host: %s

%s`, path, queryString, headerString, host, requestBody)
	request = strconv.Quote(request)
	response := fmt.Sprintf(`HTTP/2 200 OK
Content-Type: text/html
Content-Length: %d
%s

%s`, len(responseBody), responseHeaderString, responseBody)
	response = strconv.Quote(response)
	sourceType := "har"
	randTag := utils.RandStringBytes(16)
	// build flow
	flow := &schema.HTTPFlow{
		Request:    request,
		Response:   response,
		Method:     "POST",
		Url:        url,
		BodyLength: int64(len(requestBody)),
		StatusCode: 200,
		// metadata
		SourceType: sourceType,
		Tags:       randTag,
	}

	// convert flow to har entry
	entry, err := HTTPFlow2HarEntry(flow)

	// check request
	require.NoError(t, err)
	gotRequest := entry.Request
	gotResponse := entry.Response
	require.Equal(t, "POST", gotRequest.Method)
	require.Equal(t, url, gotRequest.URL)
	require.Equal(t, "HTTP/2", gotRequest.HTTPVersion)
	// sort query params
	sort.Slice(queryParams, func(i, j int) bool {
		return queryParams[i].Name < queryParams[j].Name
	})
	sort.Slice(gotRequest.QueryString, func(i, j int) bool {
		return gotRequest.QueryString[i].Name < gotRequest.QueryString[j].Name
	})
	require.Equal(t, queryParams, gotRequest.QueryString)

	// add extra header for check
	headers = append(headers,
		&HARKVPair{
			Name:  ":authority",
			Value: host,
		}, &HARKVPair{
			Name:  ":method",
			Value: "POST",
		}, &HARKVPair{
			Name:  ":path",
			Value: path,
		})
	// in http2, headers are all lower
	for i := range headers {
		headers[i].Name = strings.ToLower(headers[i].Name)
	}
	// sort headers
	sort.Slice(headers, func(i, j int) bool {
		return headers[i].Name < headers[j].Name
	})
	sort.Slice(gotRequest.Headers, func(i, j int) bool {
		return gotRequest.Headers[i].Name < gotRequest.Headers[j].Name
	})
	require.Equal(t, headers, gotRequest.Headers)
	require.Equal(t, len(requestBody), gotRequest.BodySize)
	require.Equal(t, requestBody, gotRequest.PostData.Text)

	// check response
	require.Equal(t, 200, gotResponse.StatusCode)
	require.Equal(t, "OK", gotResponse.StatusText)
	require.Equal(t, "HTTP/2", gotResponse.HTTPVersion)
	// add extra header for check
	responseHeaders = append(responseHeaders,
		&HARKVPair{
			Name:  "content-type",
			Value: "text/html",
		}, &HARKVPair{
			Name:  "content-length",
			Value: strconv.Itoa(len(responseBody)),
		})
	// in http2, headers are all lower
	for i := range responseHeaders {
		responseHeaders[i].Name = strings.ToLower(responseHeaders[i].Name)
	}
	// sort headers
	sort.Slice(responseHeaders, func(i, j int) bool {
		return responseHeaders[i].Name < responseHeaders[j].Name
	})
	sort.Slice(gotResponse.Headers, func(i, j int) bool {
		return gotResponse.Headers[i].Name < gotResponse.Headers[j].Name
	})
	require.Equal(t, responseHeaders, gotResponse.Headers)
	require.Equal(t, len(responseBody), gotResponse.BodySize)
	require.Equal(t, responseBody, gotResponse.Content.Text)

	// check metadata
	require.Equal(t, sourceType, flow.SourceType)
	require.Equal(t, randTag, flow.Tags)
}

// TestHTTPFlow2HarEntryWithFieldSelection tests HAR export field selection functionality
// Similar to Excel export test style, tests the impact of field selection on HAR export
func TestHTTPFlow2HarEntryWithFieldSelection(t *testing.T) {
	requestBody := "test request body"
	responseBody := "test response body"
	randTag := utils.RandStringBytes(16)
	fromPlugin := "test-plugin"
	duration := int64(123)
	ipAddress := "192.168.1.1"

	request := fmt.Sprintf(`POST /test HTTP/1.1
Host: example.com
Content-Type: application/json

%s`, requestBody)
	request = strconv.Quote(request)

	response := fmt.Sprintf(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Length: %d

%s`, len(responseBody), responseBody)
	response = strconv.Quote(response)

	flow := &schema.HTTPFlow{
		Request:           request,
		Response:          response,
		Method:            "POST",
		Url:               "http://example.com/test",
		StatusCode:        200,
		BodyLength:        int64(len(responseBody)),
		SourceType:        "mitm",
		Tags:              randTag,
		FromPlugin:        fromPlugin,
		Duration:          duration,
		IPAddress:         ipAddress,
		Path:              "/test",
		GetParamsTotal:    2,
		PostParamsTotal:   1,
		CookieParamsTotal: 3,
		IsWebsocket:       false,
		Payload:           "test-payload",
	}

	t.Run("only request packet fields", func(t *testing.T) {
		// Similar to Excel export test, test selecting specific fields only
		options := &HTTPFlow2HarEntryOptions{
			SelectedFields: []string{"request", "method", "url"},
		}
		entry, err := HTTPFlow2HarEntry(flow, options)
		require.NoError(t, err)
		require.NotNil(t, entry)
		// Should include request body
		require.NotNil(t, entry.Request.PostData)
		require.Equal(t, requestBody, entry.Request.PostData.Text)
		// Should not include response body
		require.NotNil(t, entry.Response.Content)
		require.Equal(t, "", entry.Response.Content.Text)
	})

	t.Run("only response packet fields", func(t *testing.T) {
		options := &HTTPFlow2HarEntryOptions{
			SelectedFields: []string{"response", "status_code", "url"},
		}
		entry, err := HTTPFlow2HarEntry(flow, options)
		require.NoError(t, err)
		require.NotNil(t, entry)
		// Should not include request body
		require.Nil(t, entry.Request.PostData)
		// Should include response body
		require.NotNil(t, entry.Response.Content)
		require.Equal(t, responseBody, entry.Response.Content.Text)
	})

	t.Run("only metadata fields", func(t *testing.T) {
		// Similar to Excel export test, test selecting metadata fields only
		options := &HTTPFlow2HarEntryOptions{
			SelectedFields: []string{"tags", "from_plugin", "duration", "iP_address", "path"},
		}
		entry, err := HTTPFlow2HarEntry(flow, options)
		require.NoError(t, err)
		require.NotNil(t, entry)
		// Should not include request and response body
		require.Nil(t, entry.Request.PostData)
		require.NotNil(t, entry.Response.Content)
		require.Equal(t, "", entry.Response.Content.Text)
		// Should include selected metadata fields
		require.NotNil(t, entry.MetaData)
		require.Equal(t, randTag, entry.MetaData.Tags)
		require.Equal(t, fromPlugin, entry.MetaData.FromPlugin)
		require.Equal(t, duration, entry.MetaData.Duration)
		require.Equal(t, ipAddress, entry.MetaData.IPAddress)
		require.Equal(t, "/test", entry.MetaData.Path)
		// Unselected fields should be empty
		require.Equal(t, "", entry.MetaData.SourceType)
		require.Equal(t, "", entry.MetaData.Payload)
	})

	t.Run("include parameter statistics fields", func(t *testing.T) {
		// Test parameter statistics field selection
		options := &HTTPFlow2HarEntryOptions{
			SelectedFields: []string{"get_params_total", "path", "url"},
		}
		entry, err := HTTPFlow2HarEntry(flow, options)
		require.NoError(t, err)
		require.NotNil(t, entry)
		require.NotNil(t, entry.MetaData)
		// When get_params_total is selected, should include all parameter statistics
		require.Equal(t, 2, entry.MetaData.GetParamsTotal)
		require.Equal(t, 1, entry.MetaData.PostParamsTotal)
		require.Equal(t, 3, entry.MetaData.CookieParamsTotal)
	})
}
