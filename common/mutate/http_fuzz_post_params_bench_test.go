package mutate

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

var postCommonParamsCountSink int

func buildPostCommonParamsBenchmarkRequest(bodyBytes int) (*FuzzHTTPRequest, error) {
	body := bytes.Repeat([]byte("r"), bodyBytes)
	packet := append(
		[]byte(fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n", len(body))),
		body...,
	)
	return NewFuzzHTTPRequest(packet)
}

func TestGetPostCommonParamsPreservesBodyAcrossCalls(t *testing.T) {
	request, err := buildPostCommonParamsBenchmarkRequest(64 * 1024)
	if err != nil {
		t.Fatal(err)
	}
	for call := 0; call < 3; call++ {
		if got := len(request.GetPostCommonParams()); got != 1 {
			t.Fatalf("call %d returned %d params, want 1", call, got)
		}
	}
}

type postParamSignature struct {
	position   string
	param      string
	param2nd   string
	paramValue string
	path       string
	gpath      string
}

func postParamSignatures(params []*FuzzHTTPRequestParam) []postParamSignature {
	result := make([]postParamSignature, 0, len(params))
	for _, param := range params {
		result = append(result, postParamSignature{
			position:   string(param.position),
			param:      fmt.Sprintf("%#v", param.param),
			param2nd:   fmt.Sprintf("%#v", param.param2nd),
			paramValue: fmt.Sprintf("%#v", param.paramValue),
			path:       param.path,
			gpath:      param.gpath,
		})
	}
	return result
}

func legacyGetPostCommonParams(request *FuzzHTTPRequest) []*FuzzHTTPRequestParam {
	req, err := request.GetOriginHTTPRequest()
	if err == nil {
		bodyRaw := httpRequestReadBody(req)
		if bodyRaw != nil && len(bodyRaw) > 0 {
			if _, ok := utils.IsJSON(string(bytes.TrimSpace(bodyRaw))); ok {
				return request.GetPostJsonParams()
			}
		}
	}
	postParams := request.GetPostXMLParams()
	if len(postParams) <= 0 {
		postParams = request.GetPostParams()
	}
	return postParams
}

func newPostCommonParamsRequest(body []byte, contentType string) (*FuzzHTTPRequest, error) {
	packet := append(
		[]byte(fmt.Sprintf("POST / HTTP/1.1\r\nHost: example.test\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n", contentType, len(body))),
		body...,
	)
	return NewFuzzHTTPRequest(packet)
}

func TestGetPostCommonParamsMatchesLegacyControlFlow(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{name: "empty", contentType: "application/octet-stream"},
		{name: "json", contentType: "application/json", body: []byte(`{"a":1,"nested":{"b":2}}`)},
		{name: "url encoded json", contentType: "application/octet-stream", body: []byte(`%7B%22a%22%3A1%7D`)},
		{name: "xml", contentType: "application/xml", body: []byte(`<root><a>1</a><b>2</b></root>`)},
		{name: "duplicate form keys", contentType: "application/x-www-form-urlencoded", body: []byte(`a=1&a=2&b=3`)},
		{name: "json form value", contentType: "application/x-www-form-urlencoded", body: []byte(`a=%7B%22x%22%3A1%7D`)},
		{name: "base64 json form value", contentType: "application/x-www-form-urlencoded", body: []byte(`a=eyJ4IjoxfQ==`)},
		{name: "printable binary", contentType: "application/octet-stream", body: bytes.Repeat([]byte("r"), 4096)},
		{name: "non printable binary", contentType: "application/octet-stream", body: []byte{0, 1, 2, 3}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacyRequest, err := newPostCommonParamsRequest(test.body, test.contentType)
			if err != nil {
				t.Fatal(err)
			}
			candidateRequest, err := newPostCommonParamsRequest(test.body, test.contentType)
			if err != nil {
				t.Fatal(err)
			}
			want := postParamSignatures(legacyGetPostCommonParams(legacyRequest))
			got := postParamSignatures(candidateRequest.GetPostCommonParams())
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("parameter mismatch:\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func BenchmarkGetPostCommonParams64KBinaryBody(b *testing.B) {
	request, err := buildPostCommonParamsBenchmarkRequest(64 * 1024)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(64 * 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		postCommonParamsCountSink = len(request.GetPostCommonParams())
	}
}
