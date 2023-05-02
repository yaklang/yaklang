package lowhttp

import (
	"testing"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCurlToHTTPRequest(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST http://baidu.com -H "Content-Type: application/json" -d '{"a":1}'`)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST / HTTP/1.`, "application/json"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequest2(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST http://baidu.com -H "Content-Type: application/json" -d '{"a":1}' -H "User-Agent: abasdfasdfasdfasf" `)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST / HTTP/1.`, "application/json", "abasdfasdfasdfasf"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequest22(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST -H "Content-Type: application/json" https://baidu.com/abcaaa -d '{"a":1}' -H "User-Agent: abasdfasdfasdfasf" `)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST`, `HTTP/1.`, "tion/json", "dfasdfasf", "abcaaa"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequest223(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST https://baidu.com/abcaaa -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST`, `HTTP/1.`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequest22311(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST https://baidu.com/abcaaa -b abc=1 -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST`, `HTTP/1.`, `Cookie`, `abc=`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequest22311_Cookie2(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST https://baidu.com/abcaaa -b abc=1 -b ccc=1 -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST`, `HTTP/1.`, `Cookie`, `abc=`, `ccc=1`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequest22311_Cookie2_AuthBasic(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST https://baidu.com/abcaaa -b abc=1 -b ccc=1 -u admin:password -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt"`)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST`, `Authorization`, "Basic", `YWRtaW46cGFzc3dvcmQ=`, `HTTP/1.`, `Cookie`, `abc=`, `ccc=1`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequest2231(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl -X POST https://baidu.com/abcaaa -H "User-Agent: abasdfasdfasdfasf" -F "filename=@/tmp/file.txt" -I`)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`HEAD`, `HTTP/1.`, "dfasdfasf", "abcaaa", "/tmp/file.txt)}}", "boundary", "multipart"}) {
		panic("curl to packet faild")
	}
}
