package mutate

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/antchfx/xmlquery"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/stretchr/testify/assert"
)

func TestNewFuzzHTTPRequest(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`, OptSource("abc"))
	if err != nil {
		test.Fail("build fuzz request failed: %s", err.Error())
	}

	reqs, err := fuzzReq.FuzzMethod("GET", "POST", "HEAD").Results()
	if err != nil {
		test.FailNow("fuzz failed: %v", err)
	}
	for _, req := range reqs {
		raw, err := utils.DumpHTTPRequest(req, true)
		if err != nil {
			return
		}
		println(string(raw))
	}

	reqs, _ = fuzzReq.FuzzPath("/{{i(1-10)}}.php").Results()
	if len(reqs) != 10 {
		test.FailNow("test fuzz path failed")
	}
	for _, req := range reqs {
		raw, err := utils.DumpHTTPRequest(req, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		_ = raw
		println(string(raw))
	}
}

func TestNewFuzzHTTPRequestFuzzCookies(t *testing.T) {
}

func TestNewFuzzHTTPRequestFuzzHeader(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	fuzzReq.GetCookieParams()
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.FuzzHTTPHeader("X-COST{{i(1-10)}}", "X-VALUE-{{i(1-10)}}").Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 100 {
		test.FailNow("fuzz http header failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestNewFuzzHTTPRequestFuzzGetQueryRaw(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.FuzzGetParamsRaw("X-VALUE-{{i(1-10)}}-FOR-QUERY").Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 10 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestNewFuzzHTTPRequestFuzzGetQuery(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.FuzzGetParams("a{{i(1-10)}}", "b{{i(1-10)}}").Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 100 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestNewFuzzHTTPRequestFuzzGetQueryChain(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.
		FuzzGetParams("a{{i(1-10)}}", "b{{i(1-10)}}").
		FuzzMethod("GET", "POST").
		FuzzGetParams("c{{i(1-3)}}", "d{{i(1-4)}}").
		FuzzGetParams("fixParam", "fixValue").
		Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 100*2*12 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestFuzzHTTPRequest_FuzzPostParamsRaw(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.
		FuzzGetParams("a{{i(1-10)}}", "b{{i(1-10)}}").
		FuzzMethod("POST").
		FuzzPostRaw("raw-PostBody{{i(1-4)}}").
		Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 400 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestFuzzHTTPRequest_FuzzPostParams(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.
		FuzzGetParams("a{{i(1-10)}}", "b{{i(1-10)}}").
		FuzzMethod("POST").
		FuzzPostParams("raw-PostBody{{i(1-4)}}", "{{i(1-5)}}").
		Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 2000 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestFuzzHTTPRequest_FuzzPostJsonParam(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.
		FuzzMethod("POST").
		FuzzPostJsonParams("id", "value-{{i(99-104)}}").
		FuzzPostJsonParams("id2", "value2-{{ri(5, 199)}}").
		Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 6 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestFuzzHTTPRequest_FuzzCookieParam(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.
		FuzzMethod("POST").
		FuzzPostJsonParams("id2", "value2-{{ri(5, 199)}}").
		FuzzCookie("test", "1{{i(1-4)}}").
		FuzzCookie("test2", "1{{i(1-4)}}").
		FuzzCookie("test2asdf", "1{{i(1-4)}}").
		FuzzCookie("test2asAAdf", "HACKEDPARAM{{ri}}").
		Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 4*4*4 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestFuzzHTTPRequest_GetCommonParams(t *testing.T) {
	test := assert.New(t)
	req, err := NewFuzzHTTPRequest(`
GET /?a=123&a=46&b=123 HTTP/1.1
Host: www.baidu.com

{"abc": "123", "a": 123}
`)
	if err != nil {
		test.FailNow(err.Error())
	}

	params := req.GetCommonParams()
	if len(params) != 4 {
		dump(params)
		test.FailNow("获取通用参数数量错误", len(params))
	}

	for _, p := range params {
		res, err := p.Fuzz("HACKEDPARAM{{i(1-20)}}").Results()
		if err != nil {
			test.FailNow("Fuzz failed")
		}
		for _, r := range res {
			raw, err := utils.DumpHTTPRequest(r, true)
			if err != nil {
				test.FailNow(err.Error())
			}
			println(string(raw))
		}
	}
}

func TestFuzzHTTPRequest_GetCommonParamsWithPOSTJSON(t *testing.T) {
	test := assert.New(t)
	req, err := NewFuzzHTTPRequest(`
POST / HTTP/1.1
Host: www.baidu.com

{"abc": "123", "a": 123, "c":{"q":"123"}}
`)
	if err != nil {
		test.FailNow(err.Error())
	}

	params := req.GetCommonParams()
	if len(params) != 4 {
		dump(params)
		test.FailNow("获取通用参数数量错误", len(params))
	}

	for _, p := range params {
		res, err := p.Fuzz("HACKEDPARAM{{i(1-20)}}").Results()
		if err != nil {
			test.FailNow("Fuzz failed")
		}
		for i, r := range res {
			raw, err := utils.DumpHTTPRequest(r, true)
			if err != nil {
				test.FailNow(err.Error())
			}
			expected := fmt.Sprintf("HACKEDPARAM%d", i+1)
			if !bytes.Contains(raw, []byte(expected)) {
				test.FailNow(fmt.Sprintf("%d FAILED: not found HACKEDPARAM%d\n%s", i, i+1, raw))
			}
		}
	}
}

func TestFuzzHTTPRequest_GetCommonParamsWithBase64(t *testing.T) {
	test := assert.New(t)
	req, err := NewFuzzHTTPRequest(`
GET /?a=cXdl&b=enhjdg%3D%3D HTTP/1.1
Host: www.baidu.com

a=cXdl&b=enhjdg%3D%3D`)
	if err != nil {
		test.FailNow(err.Error())
	}

	params := req.GetCommonParams()
	if len(params) != 8 {
		dump(params)
		test.FailNow("获取通用参数数量错误", len(params))
	}

	for _, p := range params {
		res, err := p.Fuzz("HACKEDPARAM{{i(1-20)}}").Results()
		if err != nil {
			test.FailNow("Fuzz failed")
		}
		for i, r := range res {
			raw, err := utils.DumpHTTPRequest(r, true)
			if err != nil {
				test.FailNow(err.Error())
			}
			rawStr := fmt.Sprintf("HACKEDPARAM%d", i+1)
			expected := codec.EncodeBase64(rawStr)
			expectedUrlEncoded := codec.QueryEscape(expected)
			if !bytes.Contains(raw, []byte(expected)) && !bytes.Contains(raw, []byte(expectedUrlEncoded)) && !bytes.Contains(raw, []byte(rawStr)) {
				test.FailNow(fmt.Sprintf("%d FAILED: not found HACKEDPARAM%d\n%s", i, i+1, raw))
			}
		}
	}
}

func TestFuzzHTTPRequest_GetCommonParamsForPostJson(t *testing.T) {
	test := assert.New(t)
	req, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com

{"a": {"c": "d", "e": {"f": "g"} }, "b": 2}
`)
	if err != nil {
		test.FailNow(err.Error())
	}

	params := req.GetCommonParams()
	if len(params) != 5 {
		dump(params)
		test.FailNow("获取通用参数数量错误", len(params))
	}

	for _, p := range params {
		res, err := p.Fuzz("HACKEDPARAM{{i(1-20)}}").Results()
		if err != nil {
			test.FailNow("Fuzz failed")
		}
		for _, r := range res {
			raw, err := utils.DumpHTTPRequest(r, true)
			if err != nil {
				test.FailNow(err.Error())
			}
			println(string(raw))
		}
	}
}

func TestFuzzHTTPRequest_GetCommonParamsForCookie(t *testing.T) {
	test := assert.New(t)
	req, err := NewFuzzHTTPRequest(`
GET /?a=123&a=46&b=123 HTTP/1.1
Host: www.baidu.com
Cookie: testCookie=13;

{"abc": "123", "a": 123}
`)
	if err != nil {
		test.FailNow(err.Error())
	}

	params := req.GetCommonParams()
	if len(params) != 5 {
		dump(params)
		test.FailNow("获取通用参数数量错误", len(params))
	}

	for _, p := range params {
		res, err := p.Fuzz("HACKEDPARAM{{i(1-20)}}").Results()
		if err != nil {
			test.FailNow("Fuzz failed")
		}
		for index, r := range res {
			raw, err := utils.DumpHTTPRequest(r, true)
			if err != nil {
				test.FailNow(err.Error())
			}
			if !bytes.Contains(raw, []byte("HACKEDPARAM")) {
				test.FailNow(fmt.Sprintf("FUZZ PARAM FAILED:  %d %v[%v]", index, string(p.position), p.Name()))
			}
			println(string(raw))
		}
	}
}

func TestFuzzHTTPRequest_FuzzPostJsonTypedParam(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`
GET / HTTP/1.1
Host: www.baidu.com

{"intTest": 12, "TsFloat": 1.2565}
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.
		FuzzMethod("POST").
		FuzzPostJsonParams("id", "value-{{i(99-104)}}").
		FuzzPostJsonParams("id2", "value2-{{ri(5, 199)}}").
		FuzzPostJsonParams("intTest", "{{ri}}").
		FuzzPostJsonParams("TsFloat", "1.111{{ri}}aaa").
		Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 6 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestFuzzHTTPRequest_FuzzFormEncoded(t *testing.T) {
	test := assert.New(t)
	fuzzReq, err := NewFuzzHTTPRequest(`POST / HTTP/1.1
Host: localhost:8000
User-Agent: Mozilla/5.0 (X11; Ubuntu; Linux i686; rv:29.0) Gecko/20100101 Firefox/29.0
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8
Accept-Language: en-US,en;q=0.5
Accept-Encoding: gzip, deflate
Cookie: __atuvc=34%7C7; permanent=0; _gitlab_session=226ad8a0be43681acf38c2fab9497240; __profilin=p%3Dt; request_method=GET
Connection: keep-alive
Content-Type: multipart/form-data; boundary=---------------------------9051914041544843365972754266
Content-Length: 554

-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="text"

text default
-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="text2"

text defaultads
-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="text"

text defaultadsfasdf
-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="file1"; filename="a.txt"
Content-Type: text/plain

Content of a.txt.

-----------------------------9051914041544843365972754266
Content-Disposition: form-data; name="file2"; filename="a.html"
Content-Type: text/html

<!DOCTYPE html><title>Content of a.html.</title>

-----------------------------9051914041544843365972754266--
`)
	if err != nil {
		test.FailNow("build fuzz request failed: %s", err.Error())
		return
	}

	req, err := fuzzReq.
		FuzzMethod("POST").
		FuzzFormEncoded("testvalue", "123{{i(1-2)}}").
		Results()
	if err != nil {
		test.FailNow(err.Error())
	}
	if len(req) != 2 {
		test.FailNow("fuzz http get query failed", len(req))
	}

	for _, r := range req {
		raw, err := utils.DumpHTTPRequest(r, true)
		if err != nil {
			test.FailNow(err.Error())
		}
		println(string(raw))
	}
}

func TestFuzzHTTPRequest_FuzzPostRaw(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.baidu.com
`)
	if err != nil {
		t.FailNow()
		return
	}
	freqIf := freq.FuzzUploadKVPair("test", "123123").FuzzUploadKVPair("test123", "123123").FuzzUploadKVPair("121aaa123test", "123asdfa123")
	freqIf.Show() //
}

func TestFuzzHTTPRequest_FuzzPostRaw1(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`POST /?a=1&b=2 HTTP/1.1
Host: www.baidu.com

c=1&d=1
`)
	if err != nil {
		t.FailNow()
		return
	}
	// rsp, err := freq.ExecFirst()
	// reqRaw, err := utils.DumpHTTPRequest(rsp.Request, true)
	// println(string(reqRaw))
	freq.GetCommonParams()[0].Fuzz("aaa").ExecFirst()
	// freq.fuzzGetParams("a", "1")
	println(len(freq.GetCommonParams()))
	rsp, err := freq.ExecFirst()
	if err != nil {
		t.FailNow()
		return
	}
	_ = rsp
	println(len(freq.GetCommonParams()))
}

func TestFuzzHTTPRequest_FuzzPostRawPathAppend(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`POST /abcc?a=1&b=2 HTTP/1.1
Host: www.baidu.com

c=1&d=1
`)
	if err != nil {
		t.FailNow()
		return
	}
	freq, _ = freq.FuzzPathAppend("/Hello11111").GetFirstFuzzHTTPRequest()
	if !strings.Contains(freq.GetUrl(), "/abcc/Hello111") {
		panic(1)
	}
}

func TestFuzzHTTPRequest_FuzzHeader(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`POST /abcc?a=1&b=2 HTTP/1.1
Host: www.baidu.com

c=1&d=1
`)
	if err != nil {
		t.FailNow()
		return
	}
	freq, _ = freq.GetHeaderParamByName("Header111").Fuzz("Hasdfasdfsadf").Show().GetFirstFuzzHTTPRequest()
	if !strings.Contains(freq.GetHeader("Header111"), "dfs") {
		panic(1)
	}
}

func TestNewFuzzHTTPRequestFuzzGetParams(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`POST /abcc?a=1&b=2 HTTP/1.1
Host: www.baidu.com

c=1&d=1
`)
	if err != nil {
		t.FailNow()
		return
	}
	res := freq.FuzzHTTPHeader("Header111", "dfs").FirstHTTPRequestBytes()
	// fparam.Show()
	// res, _ := freq.FuzzGetParams("a", "1")
	// res.
	//	res.RequestRaw
	if !strings.Contains(string(res), "dfs") {
		panic(1)
	}
}

func TestNewFuzzHTTPRequest2(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`GET /abc?a=1 HTTP/1.1
Host: www.baidu.com

c=1&d=1`, OptHTTPS(true))
	if err != nil {
		panic(err)
	}
	freq.FuzzPath("1111", "1123123123").FuzzPath("1111", "1123123123", "123123123123123123123").Exec()
}

func TestNewFuzzHTTPRequest2_1(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`GET /abc?a=1 HTTP/1.1
Host: www.baidu.com

c=1&d=1`, OptHTTPS(true), OptSource("abc"))
	if err != nil {
		panic(err)
	}
	res, err := freq.FuzzPath("1111", "1123123123").FuzzPath("1111", "1123123123", "123123123123123123123").Exec()
	if err != nil {
		panic(err)
	}
	for r := range res {
		spew.Dump(r.Source)
	}
}

func TestNewMustFuzzHTTPRequestJP(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`GET /abc?a={"a":1} HTTP/1.1
Host: www.baidu.com

`)
	if err != nil {
		panic(1)
	}
	for _, r := range freq.GetCommonParams() {
		r.Fuzz("ccc").Show()
	}

	freq, err = NewFuzzHTTPRequest(`POST /abc?a=1 HTTP/1.1
Host: www.baidu.com

b={"c":123}`)
	if err != nil {
		panic(1)
	}
	for _, r := range freq.GetCommonParams() {
		r.Fuzz("ccc").Show()
	}
}

func TestRecursiveXMLNode(t *testing.T) {
	t.Run("soap", func(t *testing.T) {
		rootNode, err := xmlquery.Parse(strings.NewReader(`<?xml version="1.0"?>
<soap:Envelope
xmlns:soap="http://www.w3.org/2003/05/soap-envelope/"
soap:encodingStyle="http://www.w3.org/2003/05/soap-encoding">
<soap:Header>
<TestHeader></TestHeader>
</soap:Header>

<soap:Body>
	<m:GetPrice xmlns:m="https://www.w3schools.com/prices">
	<m:Item>Apples</m:Item>
	</m:GetPrice>
	<soap:Fault>
		<Error></Error>
	</soap:Fault>
</soap:Body>
</soap:Envelope>`))
		if err != nil {
			t.Fatal(err)
		}

		count, want := 0, 2
		RecursiveXMLNode(rootNode, func(node *xmlquery.Node) {
			count++
		})
		if count != want {
			t.Fatalf("want %d nodes, but got %d", want, count)
		}
	})

	t.Run("normal", func(t *testing.T) {
		rootNode, err := xmlquery.Parse(strings.NewReader(`<?xml version="1.0"?>
<bookstore>
  <book>
    <title lang="en">Harry Potter</title>
    <author>J K. Rowling</author>
    <year>2005</year>
    <price>29.99</price>
  </book>
  <book>
    <title lang="en">English Book</title>
    <author>Lang</author>
    <year>2000</year>
    <price>1.99</price>
  </book>
</bookstore>`))
		if err != nil {
			t.Fatal(err)
		}

		count, want := 0, 11
		RecursiveXMLNode(rootNode, func(node *xmlquery.Node) {
			count++
		})
		if count != want {
			t.Fatalf("want %d nodes, but got %d", want, count)
		}
	})
}

func TestGetXpathFromNode(t *testing.T) {
	rootNode, err := xmlquery.Parse(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
	<bookstore>
	<book>
	  <title lang="en">Harry Potter</title>
	  <author>J K. Rowling</author>
	  <year>2005</year>
	  <price>29.99</price>
	</book>
	<book>
	  <title lang="en">English Book</title>
	  <author>Hello</author>
	  <year>2000</year>
	  <price>19.99</price>
	</book>
</bookstore>`))
	if err != nil {
		t.Fatal(err)
	}

	nodes, _ := xmlquery.QueryAll(rootNode, "//author")
	for _, node := range nodes {
		xpath := GetXpathFromNode(node)
		if n, err := xmlquery.Query(rootNode, xpath); err != nil {
			t.Fatalf("can't find node[%s]", xpath)
			break
		} else if n != node {
			t.Fatalf("node[%s] not equal", xpath)
			break
		}
	}
}

func TestFuzzRequestParam_JSON(t *testing.T) {
	freq, err := NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.example.com

{"a": {"b": 1, "c": [1, {"d": 222}]}}`)
	if err != nil {
		t.FailNow()
		return
	}
	/*

	   === RUN   TestFuzzRequestParam_JSON
	   (string) (len=3) "$.a"
	   (string) (len=5) "$.a.b"
	   (string) (len=5) "$.a.c"
	   (string) (len=8) "$.a.c[0]"
	   (string) (len=8) "$.a.c[1]"
	   (string) (len=10) "$.a.c[1].d"

	*/
	numCount := 0
	otherCount := 0
	for _, o := range freq.GetCommonParams() {
		p := o.GetPostJsonPath()
		if p == "" {
			continue
		}
		if o.FirstValueIsNumber() {
			fmt.Printf("NUMBER: %v", o.String())
			numCount++
		} else {
			fmt.Printf("EXCLUDE %v", o.String())
			otherCount++
		}
	}
	spew.Dump(numCount, otherCount)
	test := assert.New(t)
	test.Equal(numCount, 3)
	test.Equal(otherCount, 3)
}

func TestFuzzRequestPath(t *testing.T) {
	type TestCase struct {
		request string
		path    []string

		testName string
	}
	testCase := TestCase{
		request: `GET /a/b/c HTTP/1.1
HOST: www.example.com`,
		path:     []string{"/test/b/c", "/a/test/c", "/a/b/test"},
		testName: "test",
	}
	request, err := NewFuzzHTTPRequest(testCase.request)
	if err != nil {
		panic(err)
	}
	param := request.GetPathBlockParams()[0]
	results, err := param.Fuzz(testCase.testName).Results()
	if err != nil {
		panic(err)
	}
	if len(results) != 3 {
		t.Fatalf("request number not match,except %v", 3)
	}
	for i, result := range results {
		if result.RequestURI != testCase.path[i] {
			t.Fatalf("request uri not match,except %v", testCase.path[i])
		}
	}
}
