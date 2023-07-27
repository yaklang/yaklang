package lowhttp

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"testing"
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

func TestCurlToHTTPRequestDataRaw(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl 'https://api.github.com/_private/browser/stats' \
  -b 'b=xxxxxx;xxx' \
  -H 'cookie: _octo=222; xxx=333;a=222' \
  -H 'user-agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36' \
  --data-raw 'abcd' \
  --compressed`)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(req))
	raw, err := ParseBytesToHttpRequest(req)
	if err != nil {
		return
	}
	rawBody, err := ioutil.ReadAll(raw.Body)
	if err != nil {
		return
	}
	if string(rawBody) != "abcd" {
		panic("curl to packet faild")
	}
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST`, `HTTP/1.`, "xxxxx", "_octo=222"}) {
		panic("curl to packet faild")
	}
}

func TestCurlToHTTPRequestAOrAgent(t *testing.T) {
	req, err := CurlToHTTPRequest(`curl 'https://8.8.8.8/api/graphql/QueryRiskList' \
  -H 'user-agent: -H setting' \
  -H 'authority: audio-consideration-rc2ldz.cn.goofy.app' \
  -A '-A setting' \
  --data-raw $'{"query":"query QueryRiskList($req: QueryRiskListReqInput\u0021) {\\n  QueryRiskList(req: $req) {\\n    Data\\n    TotalCount\\n  }\\n}\\n","variables":{"req":{"Filters":[{"FieldName":"basic_info.source","DataType":"String","Operator":"IN","Value":"[\\"BLACKBOX\\"]"},{"FieldName":"basic_info.created_at","DataType":"Int","Operator":"GE","Value":"1689004800"},{"FieldName":"basic_info.created_at","DataType":"Int","Operator":"LE","Value":"1689091199"},{"FieldName":"basic_info.status","DataType":"String","Operator":"IN","Value":"[\\"PENDING\\"]"},{"FieldName":"basic_info.business_tree_id","DataType":"Int","Operator":"EQ","Value":"6"},{"FieldName":"basic_info.risk_vuln_type","DataType":"String","Operator":"EQ","Value":"\\"auth_bypass\\""}],"Category":"ALL","CurrentPage":"1","PerPageItems":"20","OrderField":"basic_info.created_at","OrderType":"DESC"}}}' \
  --compressed`)
	if err != nil {
		panic(err)
	}
	println(string(req))
	if !utils.StringContainsAllOfSubString(string(req), []string{`POST`, `HTTP/1.`, "-A setting"}) {
		panic("curl to packet faild")
	}
}
