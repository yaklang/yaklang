package openapigen

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func TestExtractQueryParams(t *testing.T) {
	params := extractQueryParams("/api/v1/brute/123?test=1&test2=2")
	spew.Dump(params)
	if len(params) == 2 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 2, len(params))
	}
}

func TestShrinkPath(t *testing.T) {
	after, params, _, _ := shrinkPath("/api/v1/brute/123", "/api/v1/brute/112")
	spew.Dump(after, params)
	if after == "/api/v1/brute/{bruteId}" {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", "/api/v1/brute/{bruteId}", after)
	}

	if len(params) == 1 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 1, len(params))
	}
}

func TestShrinkPath_1(t *testing.T) {
	after, params, _, _ := shrinkPath("/api/v1/brute/123/list", "/api/v1/brute/112/list")
	spew.Dump(after, params)
	if after == "/api/v1/brute/{bruteId}/list" {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", "/api/v1/brute/{bruteId}/list", after)
	}

	if len(params) == 1 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 1, len(params))
	}
}

func TestShrinkPath_2(t *testing.T) {
	after, params, _, _ := shrinkPath("/api/v1/brute/123/1", "/api/v1/brute/112/2")
	spew.Dump(after, params)
	if after == "/api/v1/brute/{bruteId}/{bruteId2}" {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", "/api/v1/brute/{bruteId}/list", after)
	}

	if len(params) == 2 {
		t.Log("ok")
	} else {
		t.Errorf("want %v, got %v", 1, len(params))
	}
}

func TestShrinkPath2(t *testing.T) {
	after, params, isSame, _ := shrinkPath("/api/v1/brute/123", "/api/v1/brute/123")
	assert.Equal(t, isSame, true)
	test := assert.New(t)
	test.Equal(after, "/api/v1/brute/123")
	test.Equal(len(params), 0)
}

func TestBasicUse(t *testing.T) {
	path, item, err := HttpFlowToOpenAPIStruct("", nil, []byte(`GET /v1/abc/asdfasdf/1
Host: www.example.com
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	path, item, err = HttpFlowToOpenAPIStruct(path, item, []byte(`GET /v1/abc/asdfasdf/2
Host: www.example.com`), []byte(`HTTP/1.1 302 Per
Content-Length: 1

1`))
	spew.Dump(path)
	raw, err := item.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = openapiyaml.JSONToYAML(raw)
	fmt.Println(string(raw))

	if !strings.Contains(string(raw), `asdfasdfId`) {
		t.Fatal("not contains asdfasdfId")
	}
}

func TestGenerate(t *testing.T) {
	input := make(chan *BasicHTTPFlow, 100)
	input <- &BasicHTTPFlow{
		Request: []byte(`GET /v1/abc/asdfasdf/1
Host: www.example.com`),
		Response: []byte("HTTP/1.1 302 Per\nContent-Length: 1\n\n1"),
	}
	input <- &BasicHTTPFlow{
		Request: []byte(`GET /v1/abc/asdfasdf/2
Host: www.example.com`),
		Response: []byte("HTTP/1.1 302 Per\nContent-Length: 1\n\n1"),
	}
	input <- &BasicHTTPFlow{
		Request: []byte(`GET /v1/abc/ddd/aaa21
Host: www.example.com`),
		Response: []byte("HTTP/1.1 302 Per\nContent-Length: 1\n\n1"),
	}
	input <- &BasicHTTPFlow{
		Request: []byte(`GET /v1/abc/eee/2?id=2&ca=1
Host: www.example.com`),
		Response: []byte("HTTP/1.1 302 Per\nContent-Length: 1\n\n1"),
	}
	input <- &BasicHTTPFlow{
		Request: []byte(`GET /v1/abc/eee/5
Host: www.example.com`),
		Response: []byte("HTTP/1.1 302 Per\nContent-Length: 1\n\n1"),
	}
	input <- &BasicHTTPFlow{
		Request: []byte(`GET /v1/abc/eee/5?id=1
Host: www.example.com`),
		Response: []byte("HTTP/1.1 302 Per\nContent-Length: 1\n\n1"),
	}
	close(input)

	raw, err := generate(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = openapiyaml.JSONToYAML(raw)
	fmt.Println(string(raw))

	if utils.MatchAllOfSubString(
		string(raw),
		`/v1/abc/eee/5:`,
		`/v1/abc/ddd/{dddId}:`,
		`/v1/abc/asdfasdf/{asdfasdfId}:`,
		`schem`, "type: integer",
		"content:", `application/json:`,
	) {
		t.Log("ok")
	} else {
		t.Fatal("not match")
	}
}
