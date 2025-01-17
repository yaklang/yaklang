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
)

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
	responseHeaderString := strings.Join(lo.Map(headers, func(item *HARKVPair, _ int) string {
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
	// build flow
	flow := &schema.HTTPFlow{
		Request:    request,
		Response:   response,
		Method:     "POST",
		Url:        url,
		BodyLength: int64(len(requestBody)),
		StatusCode: 200,
	}

	// convert flow to har entry
	entry, err := HTTPFlow2HarEntry(flow)

	// check request
	require.NoError(t, err)
	gotRequest := entry.Request
	gotResponse := entry.Response
	_ = gotResponse
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
	require.Equal(t, headers, gotRequest.Headers)
	require.Equal(t, len(responseBody), gotResponse.BodySize)
	require.Equal(t, responseBody, gotResponse.Content.Text)
}
