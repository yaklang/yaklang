package mutate_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func initDB() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()

	_ = yaklang.New()
	_ = yak.NewScriptEngine(1)
}

func init() {
	initDB()
}

/*
type github.com/yaklang/yaklang/common/mutate.(FuzzHTTPRequest) struct {
  Fields(可用字段):
      Opts: []mutate.BuildFuzzHTTPRequestOption
  StructMethods(结构方法/函数):
  PtrStructMethods(指针结构方法/函数):
      func Exec(v1 ...func HttpPoolConfigOption(v1: *mutate.httpPoolConfig) ) return(chan *mutate._httpResult, error)
      func ExecFirst(v1 ...func HttpPoolConfigOption(v1: *mutate.httpPoolConfig) ) return(*mutate._httpResult, error)
      func FirstFuzzHTTPRequest() return(*mutate.FuzzHTTPRequest)
      func FirstHTTPRequestBytes() return([]uint8)
      func FuzzCookie(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzCookieRaw(v1: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzFormEncoded(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzGetJsonPathParams(v1: interface {}, v2: string, v3: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzGetParams(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzGetParamsRaw(v1 ...string) return(mutate.FuzzHTTPRequestIf)
      func FuzzHTTPHeader(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzMethod(v1 ...string) return(mutate.FuzzHTTPRequestIf)
      func FuzzPath(v1 ...string) return(mutate.FuzzHTTPRequestIf)
      func FuzzPathAppend(v1 ...string) return(mutate.FuzzHTTPRequestIf)
      func FuzzPostJsonParams(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzPostJsonPathParams(v1: interface {}, v2: string, v3: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzPostParams(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzPostRaw(v1 ...string) return(mutate.FuzzHTTPRequestIf)
      func FuzzUploadFile(v1: interface {}, v2: interface {}, v3: []uint8) return(mutate.FuzzHTTPRequestIf)
      func FuzzUploadFileName(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func FuzzUploadKVPair(v1: interface {}, v2: interface {}) return(mutate.FuzzHTTPRequestIf)
      func GetBody() return([]uint8)
      func GetBytes() return([]uint8)
      func GetCommonParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetContentType() return(string)
      func GetCookieParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetFirstFuzzHTTPRequest() return(*mutate.FuzzHTTPRequest, error)
      func GetGetQueryParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetHeader(v1: string) return(string)
      func GetHeaderKeys() return([]string)
      func GetHeaderParamByName(v1: string) return(*mutate.FuzzHTTPRequestParam)
      func GetHeaderParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetHeaderValues() return([]string)
      func GetMethod() return(string)
      func GetOriginHTTPRequest() return(*http.Request, error)
      func GetPath() return(string)
      func GetPathAppendParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetPathBlockParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetPathParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetPathRawParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetPathWithQuery() return(string)
      func GetPostJsonParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetPostParams() return([]*mutate.FuzzHTTPRequestParam)
      func GetPostQuery() return(string)
      func GetPostQueryKeys() return([]string)
      func GetPostQueryValue(v1: string) return(string)
      func GetPostQueryValues() return([]string)
      func GetQueryKeys() return([]string)
      func GetQueryRaw() return(string)
      func GetQueryValue(v1: string) return(string)
      func GetQueryValues() return([]string)
      func GetRequestURI() return(string)
      func GetUrl() return(string)
      func IsBodyFormEncoded() return(bool)
      func IsBodyJsonEncoded() return(bool)
      func IsBodyUrlEncoded() return(bool)
      func IsEmptyBody() return(bool)
      func ParamsHash() return(string, error)
      func Repeat(v1: int) return(mutate.FuzzHTTPRequestIf)
      func Results() return([]*http.Request, error)
      func Show() return(mutate.FuzzHTTPRequestIf)
}
*/

type base struct {
	inputPacket                 string
	code                        string
	expectPacketNum             int
	expectKeywordInOutputPacket []string
	expectRegexpInOutputPacket  []string
	debug                       bool
	disableEncode               bool
	friendlyDisplay             bool
}

func testCaseCheck(base base) func(t *testing.T) {
	return func(t *testing.T) {
		test := assert.New(t)
		ctx := context.Background()
		engine := yaklang.New()
		data := base

		engine.SetVars(map[string]any{
			"request":  data.inputPacket,
			"keywords": data.expectKeywordInOutputPacket,
			"regexps":  data.expectRegexpInOutputPacket,
			"debug":    data.debug,
		})

		if data.code != "" {
			data.code = "." + strings.TrimLeft(data.code, ".")
		}
		var initCode string
		if data.disableEncode && data.friendlyDisplay {
			initCode = `result = fuzz.HTTPRequest(request,fuzz.noEncode(true),fuzz.showTag())~` + data.code
		} else if data.disableEncode {
			initCode = `result = fuzz.HTTPRequest(request,fuzz.noEncode(true))~` + data.code
		} else if data.friendlyDisplay {
			initCode = `result = fuzz.HTTPRequest(request,fuzz.showTag())~` + data.code
		} else {
			initCode = `result = fuzz.HTTPRequest(request)~` + data.code
		}

		if data.debug {
			fmt.Println("----------------OP CODE-----------------")
			fmt.Println(initCode)
			fmt.Println("----------------------------------------")
		}
		err := engine.EvalInline(ctx, initCode)
		test.NoError(err, "eval code should not fail")

		if data.debug {
			t.Log("----------------KEYWORD-----------------")
			engine.EvalInline(ctx, "dump(keywords)")
			t.Log("----------------REGEXPS-----------------")
			engine.EvalInline(ctx, "dump(regexps)")
		}

		err = engine.EvalInline(context.Background(), `raw = result.GetFirstFuzzHTTPRequest()~.GetBytes()
if debug { println(string(raw)) }
check = str.MatchAllOfSubString(raw, keywords...) || str.MatchAllOfRegexp(raw, regexps...)
expectPacketNum = result.Results()~.Len()
`)
		test.NoError(err, "eval code should not fail")

		checked, ok := engine.GetVar("check")
		test.True(ok, "should get 'check' variable")
		test.True(checked.(bool), "check should be true")

		if data.expectPacketNum > 0 {
			packetNum, ok := engine.GetVar("expectPacketNum")
			test.True(ok, "should get 'expectPacketNum' variable")
			test.Equal(data.expectPacketNum, packetNum.(int), "packet num should be equal")
		}
	}

}

func TestFuzzMethod(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "Fuzz Method",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
				code:                        `.FuzzMethod("ABC")`,
				expectKeywordInOutputPacket: []string{"ABC / HTTP/1.1\r\n"},
			},
		},
		{
			name: "Fuzz Method with multiple methods",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
				code:                        `.FuzzMethod(["a","b","c"]...)`,
				expectKeywordInOutputPacket: []string{"a / HTTP/1.1\r\n"},
				expectPacketNum:             3,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}

}

func TestFuzzGetParams(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "GET参数 默认",
			base: base{
				inputPacket: `GET /?a=MTIzNA== HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParams("b", "%25%25").FuzzGetParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA%3D%3D",
					"b=%2525%2525",
					"c=%24",
				},
				debug: true,
			},
		},
		{
			name: "GET参数 友好显示",
			base: base{
				inputPacket: `GET /?a=MTIzNA== HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParams("b", "%25%25").FuzzGetParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a={{urlescape(MTIzNA==)}}",
					"b={{urlescape(%25%25)}}",
					"c={{urlescape($)}}",
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "GET参数 禁止指定参数自动编码",
			base: base{
				inputPacket: `GET /?a=MTIzNA== HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParams("b", "%25%25").FuzzGetParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA==",
					"b=%25%25",
					"c=$",
				},
				disableEncode: true,
			},
		},
		{
			name: "GET参数 禁止编码 & 友好显示",
			base: base{
				inputPacket: `GET /?a=MTIzNA== HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParams("b", "%25%25")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA==", "b=%25%25",
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "GET参数 禁止编码 & 友好显示 2",
			base: base{
				inputPacket: `GET /?a=MTIzNA== HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParams("b", "$").FuzzGetParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA==",
					"b=$",
					"c=$",
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "GET参数 友好显示 2",
			base: base{
				inputPacket: `GET /?a=MTIzNA==&b=2 HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParams("b", "12")`,
				expectKeywordInOutputPacket: []string{
					"a={{urlescape(MTIzNA==)}}", "b=12",
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "GET参数 Raw",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParamsRaw("ccccccccccccccc")`,
				expectKeywordInOutputPacket: []string{
					"/?ccccccccccccccc",
				},
			},
		},
		{
			name: "GET参数 Packet Num",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParamsRaw("{{int(1-3)}}")`,
				expectKeywordInOutputPacket: []string{
					"/?1",
				},
				expectPacketNum: 3,
			},
		},
		{
			name: "GET 参数(JSON) vulinbox ssti case",
			base: base{
				inputPacket: `GET /expr/injection?b={%22a%22:%201} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetParams("b", ` + `{"a":"{2018-05-25}"}` + `)`,
				expectKeywordInOutputPacket: []string{
					`/expr/injection`,
					`b={"a":"{2018-05-25}"}`,
				},
				disableEncode: true,
				//debug:         true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzGetJsonPathParams(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "GET 参数(JSON) 友好显示 类型不匹配",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":"string"})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) string type 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": "123"} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", "99999")`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":"99999"})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) number type 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":99999})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) boolean type 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": false} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", true)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":true})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) json type 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": {"c":"d"}} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", {"zz":123})`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":{"zz":123}})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) null type 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": null} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", 123)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":123})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) null type 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", nil)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":null})}}`,
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "GET 参数(JSON) array type 友好显示",
			base: base{
				inputPacket: `GET /?a=[{"id": 1},{"id": 2}] HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.[0]", {"id":111})`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape([{"id":111},{"id":2}])}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) 默认显示",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a=%7B%22abc%22%3A%22string%22%7D`, // {"abc":"string"}
				},
			},
		},
		{
			name: "GET 参数(JSON) 禁止编码",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a={"abc":"string"}`,
				},
				disableEncode: true,
			},
		},
		{
			name: "GET 参数(JSON) 禁止编码 & 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a={"abc":"string"}`,
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "GET 参数(JSON) 禁止编码 & 友好显示",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("a", "$.d", "dd").FuzzGetJsonPathParams("a", "$.e", 123).FuzzGetJsonPathParams("a", "$.f", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`a={"abc":123,"d":"dd","e":123,"f":{"xx":123}}`,
				},
				disableEncode: true,
				//debug:         true,
			},
		},
		{
			name: "GET 参数(JSON) slqi case",
			base: base{
				inputPacket: `GET /user/id-json?id=%7B%22uid%22%3A1%2C%22id%22%3A%221%22%7D HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetJsonPathParams("id", "$.id", "1/**/ORDeR/**/bY/**/9-- ")`,
				expectKeywordInOutputPacket: []string{
					url.QueryEscape(`1/**/ORDeR/**/bY/**/9-- `),
				},
				debug: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzGetBase64Params(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "GET参数(Base64)",
			base: base{
				inputPacket: `GET /?a=MTIzNA==&b=2 HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64Params("a", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a=OTk5OTk%3D`,
				},
			},
		},
		{
			name: "GET参数(Base64) 友好显示",
			base: base{
				inputPacket: `GET /?a=MTIzNA==&b=2 HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64Params("a", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({{base64(99999)}})}}&b=2`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET参数(Base64) 禁止编码",
			base: base{
				inputPacket: `GET /?a=MTIzNA==&b=2 HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64Params("a", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a=OTk5OTk=&b=2`,
				},
				disableEncode: true,
			},
		},
		{
			name: "GET参数(Base64) 禁止编码 & 友好显示",
			base: base{
				inputPacket: `GET /?a=MTIzNA==&b=2 HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64Params("a", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a={{base64(99999)}}`,
					`b=2`,
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "GET参数(Base64) 禁止编码 & 友好显示 & Packet Num",
			base: base{
				inputPacket: `GET /?a=MTIzNA==&b=2 HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64Params("a", "{{int(1-3)}}")`,
				expectKeywordInOutputPacket: []string{
					`a={{base64(1)}}`,
					`b=2`,
				},
				expectPacketNum: 3,
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzGetBase64JsonPath(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "GET参数(Base64+JSON) 默认",
			base: base{
				inputPacket: `GET /acc.t1?a=ab&c=eyJkZCI6MTI1fQ%3D%3D HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64JsonPath("c", "$.dd", 1234)`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c=eyJkZCI6MTIzNH0%3D`, // {"dd":1234}
				},
			},
		},
		{
			name: "GET参数(Base64+JSON) 禁止编码",
			base: base{
				inputPacket: `GET /acc.t1?a=ab&c=eyJkZCI6MTI1fQ%3D%3D HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64JsonPath("c", "$.dd", 1234)`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c=eyJkZCI6MTIzNH0=`, // {"dd":1234}
				},
				disableEncode: true,
			},
		},
		{
			name: "GET参数(Base64+JSON) 友好显示",
			base: base{
				inputPacket: `GET /acc.t1?a=ab&c=eyJkZCI6MTI1fQ%3D%3D HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64JsonPath("c", "$.dd", 1234)`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c={{urlescape({{base64({"dd":1234})}})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "GET参数(Base64+JSON) 友好显示 2",
			base: base{
				inputPacket: `GET /acc.t1?a=ab&c=W3siaWQiOiAxfSx7ImlkIjogMn1d HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzGetBase64JsonPath("c", "$.[0]", {"xx":"bb"})`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c={{urlescape({{base64([{"xx":"bb"},{"id":2}])}})}}`,
				},
				friendlyDisplay: true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzHTTPHeader(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "Header 覆盖/追加",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzHTTPHeader("c", "123").FuzzHTTPHeader("a", "ab").FuzzHTTPHeader("a", "$")`,
				expectKeywordInOutputPacket: []string{
					`c: 123`,
					`a: $`,
					`123456`,
				},
				debug: true,
			},
		},
		{
			name: "Header 修改",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzHTTPHeader("host", "123")`,
				expectKeywordInOutputPacket: []string{
					`host: 123`,
				},
				debug: true,
			},
		},
		{
			name: "Header chunked",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzHTTPHeader("transfer-encoding", "chunked")`,
				expectKeywordInOutputPacket: []string{
					`transfer-encoding: chunked`,
				},
				debug: true,
			},
		},
		{
			name: "Header Single Key",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
host: 1.2.4.5
content-type: a/1
content-type: a/2
Content-type: a/3
Content-Type: a/4
A: aaa

123456
`,
				code: `.FuzzHTTPHeader("a", "bbb")`,
				expectKeywordInOutputPacket: []string{
					`content-type: a/1`,
					`content-type: a/2`,
					`Content-type: a/3`,
					`Content-Type: a/4`,
					`A: aaa`,
					`a: bbb`,
				},
				debug: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzPath(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "URL路径 默认",
			base: base{
				inputPacket: `GET /a/b/c/?a=ab&b=aa== HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("/1").FuzzPath("/2").FuzzPath("/3").FuzzPathAppend("/4/")`,
				expectKeywordInOutputPacket: []string{
					`/3/4/?a=ab`,
					`b=aa==`,
					`123456`,
				},
				debug: true,
			},
		},
		{
			name: "URL路径 禁止编码",
			base: base{
				inputPacket: `GET /a/b/c/?a=ab&b=aa== HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("/%24/$/%u002e")`,
				expectKeywordInOutputPacket: []string{
					`/%24/$/%u002e?a=ab`,
					`b=aa==`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "URL路径 禁止编码 2",
			base: base{
				inputPacket: `GET /a/b/c/?a=ab&b=aa== HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("/%25/$")`,
				expectKeywordInOutputPacket: []string{
					`/%25/$?a=ab&b=aa==`,
					`b=aa==`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "URL路径 禁止编码 3",
			base: base{
				inputPacket: `GET /a/b/c/?a=ab&b=aa== HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("/%25/你好")`,
				expectKeywordInOutputPacket: []string{
					`/%25/你好?a=ab&b=aa==`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "URL路径 默认",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("%25%25")`,
				expectKeywordInOutputPacket: []string{
					`/%2525%2525?a=ab`,
				},
				debug: true,
			},
		},
		{
			name: "URL路径 默认",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("$/你好")`,
				expectKeywordInOutputPacket: []string{
					`/$/`,
					codec.PathEscape("你好"),
				},
				debug: true,
			},
		},
		{
			name: "URL路径 加参数",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("$/你好?b=c")`,
				expectKeywordInOutputPacket: []string{
					`/$/`,
					codec.PathEscape("你好"),
					`b=c`,
				},
				debug: true,
			},
		},
		{
			name: "URL路径 加参数 禁止编码",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("/fastjson/json-in-query?auth={\"user\":\"admin\",\"password\":\"password\"}")`,
				expectKeywordInOutputPacket: []string{
					`/fastjson/json-in-query`,
					`auth={"user":"admin","password":"password"}`,
					`a=ab`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "URL路径 加参数 友好显示",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("/fastjson/json-in-query?auth={\"user\":\"admin\",\"password\":\"password\"}")`,
				expectKeywordInOutputPacket: []string{
					`/fastjson/json-in-query`,
					`auth={{urlescape({"user":"admin","password":"password"})}}`,
					`a=ab`,
				},
				debug:           true,
				friendlyDisplay: true,
			},
		},
		{
			name: "URL路径 加参数 友好显示",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("/fastjson/json-in-query?auth={\"user\":\"admin\",\"password\":\"password\"}")`,
				expectKeywordInOutputPacket: []string{
					`/fastjson/json-in-query`,
					`auth={{urlescape({"user":"admin","password":"password"})}}`,
					`a=ab`,
				},
				debug:           true,
				friendlyDisplay: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzCookie(t *testing.T) {
	// 对于合法的值，不会进行编码，对于 " ", "," 等特殊字符会进行编码
	// 对于不合法的值，会强制进行编码
	tests := []struct {
		name string
		base base
	}{
		{
			name: "Cookie参数 默认",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "123").FuzzCookie("e", "a,b")`,
				expectKeywordInOutputPacket: []string{
					`a=123`,
					`e="`,
					url.QueryEscape("a,b"),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数 默认",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "123").FuzzCookie("e", "a,b")`,
				expectKeywordInOutputPacket: []string{
					`a=123`,
					`e="a,b"`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "Cookie参数 默认2",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "123").FuzzCookie("a", "345").FuzzCookie("e", "a,b")`,
				expectKeywordInOutputPacket: []string{
					`a=345`,
					`e="`,
					url.QueryEscape("a,b"),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数 sqli",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "1/**/ORDeR/**/bY/**/9-- ")`,
				expectKeywordInOutputPacket: []string{
					`a="`,
					url.QueryEscape(`1/**/ORDeR/**/bY/**/9-- `),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数 sqli 2",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "1/**/UniOn/**/Select/**/md5('VLpqc'),md5('VLpqc'),md5('VLpqc'),SLeep(3)-- ")`,
				expectKeywordInOutputPacket: []string{
					`a="1/**/UniOn/**/Select/**/md5('VLpqc'),md5('VLpqc'),md5('VLpqc'),SLeep(3)-- "`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "Cookie参数 xss",
			base: base{
				inputPacket: `GET /xss/cookie/name?skip=1 HTTP/1.1
Host: 127.0.0.1:8080
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
Cookie: xCname=UserAdmin

`,
				code: ".FuzzCookie(`xCname`, \"1`;</SCrIpT><Img id='jsmxNLlB' src=1 onerror='prompt(1)'><ScRiPT>`\")",
				expectKeywordInOutputPacket: []string{
					url.QueryEscape("1`;</SCrIpT><Img id='jsmxNLlB' src=1 onerror='prompt(1)'><ScRiPT>`"),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数 默认4",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "%2525")`,
				expectKeywordInOutputPacket: []string{
					`a=%2525`,
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数 禁止编码",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "123").FuzzCookie("e", "a,b")`,
				expectKeywordInOutputPacket: []string{
					`a=123`,
					`e="a,b"`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "Cookie参数 友好显示",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "123").FuzzCookie("e", "a,b").FuzzCookie("f", "a;b")`,
				expectKeywordInOutputPacket: []string{
					`a=123`,
					`e="a,b"`,
					`f={{urlescape(a;b)}}`,
				},
				debug:           true,
				friendlyDisplay: true,
			},
		},
		{
			name: "Cookie参数 禁止编码 && 友好显示 无法限制不合法的字符",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzCookie("a", "123").FuzzCookie("e", "a,b").FuzzCookie("f", "a;b")`,
				expectKeywordInOutputPacket: []string{
					`a=123`,
					`e="a,b"`,
					`f=` + url.QueryEscape(`a;b`),
				},
				debug:           true,
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "Cookie参数 fix bug",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=123%7B%22cur%22%3A%22HKD%22%7D;b=123%7B%22cur%22%3A%22HKD%22%7D;

`,
				code: `.FuzzCookie("a", "test")`,
				expectKeywordInOutputPacket: []string{
					`a=test`,
					`b=123%7B%22cur%22%3A%22HKD%22%7D`,
				},
				debug: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzCookieBase64(t *testing.T) {
	// 如果一个 Cookie Value 的值是 base64 编码的，应该对原值不进行任何的处理
	// 比如 "a,b" 不应该添加双引号后再进行 base64 编码
	tests := []struct {
		name string
		base base
	}{
		{
			name: "Cookie参数(Base64) 默认",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,

				code: `.FuzzCookieBase64("a", "%25").FuzzCookieBase64("b", "$")`,
				expectKeywordInOutputPacket: []string{
					`a=` + codec.EncodeBase64("%25"),
					`b=` + codec.EncodeBase64("$"),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数(Base64) 默认 2",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,

				code: `.FuzzCookieBase64("a", "123").FuzzCookieBase64("b", 123).FuzzCookieBase64("c", true).FuzzCookieBase64("d", "\"123\"").FuzzCookieBase64("e","a b").FuzzCookieBase64("f","c,d")`,
				expectKeywordInOutputPacket: []string{
					`a=` + codec.EncodeBase64("123"),
					`b=` + codec.EncodeBase64("123"),
					`c=` + codec.EncodeBase64("true"),
					`d=` + codec.EncodeBase64(`"123"`),
					`e=` + codec.EncodeBase64(`a b`),
					`f=` + codec.EncodeBase64(`c,d`),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数(Base64) 友好显示",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,

				code: `.FuzzCookieBase64("a", "123").FuzzCookieBase64("b", 123).FuzzCookieBase64("c", true).FuzzCookieBase64("d","\"123\"").FuzzCookieBase64("e","a b").FuzzCookieBase64("f","a,b")`,
				expectKeywordInOutputPacket: []string{
					`a={{base64(123)}}`,
					`b={{base64(123)}}`,
					`c={{base64(true)}}`,
					`d={{base64("123")}}`,
					`e={{base64(a b)}}`, // 不应该添加双引号
					`f={{base64(a,b)}}`, // 不应该添加双引号
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "Cookie参数(Base64) 禁止编码",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,

				code: `.FuzzCookieBase64("a", "123").FuzzCookieBase64("b", 123).FuzzCookieBase64("c", true).FuzzCookieBase64("d","\"123\"").FuzzCookieBase64("e","a b")`,
				expectKeywordInOutputPacket: []string{
					`a=` + codec.EncodeBase64("123"),
					`b=` + codec.EncodeBase64("123"),
					`c=` + codec.EncodeBase64("true"),
					`d=` + codec.EncodeBase64(`"123"`),
					`e=` + codec.EncodeBase64(`a b`), // 不应该添加双引号
				},
				disableEncode: true,
				debug:         true,
			},
		},
		{
			name: "Cookie参数(Base64) 禁止编码 && 友好显示 append",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

`,

				code: `.FuzzCookieBase64("a", "123").FuzzCookieBase64("b", 123).FuzzCookieBase64("c", true).FuzzCookieBase64("d","\"123\"").FuzzCookieBase64("e","a b")`,
				expectKeywordInOutputPacket: []string{
					`a={{base64(123)}}`,
					`b={{base64(123)}}`,
					`c={{base64(true)}}`,
					`d={{base64("123")}}`,
					`e={{base64(a b)}}`, // 不应该添加双引号
				},
				friendlyDisplay: true,
				disableEncode:   true,
				debug:           true,
			},
		},
		{
			name: "Cookie参数(Base64) 禁止编码 && 友好显示 replace",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=1; b=2; c=3; d="4"; e="5"

`,

				code: `.FuzzCookieBase64("a", "123").FuzzCookieBase64("b", 123).FuzzCookieBase64("c", true).FuzzCookieBase64("d","\"123\"").FuzzCookieBase64("e","a b")`,
				expectKeywordInOutputPacket: []string{
					`a={{base64(123)}}`,
					`b={{base64(123)}}`,
					`c={{base64(true)}}`,
					`d={{base64("123")}}`,
					`e={{base64(a b)}}`,
				},
				friendlyDisplay: true,
				disableEncode:   true,
				debug:           true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}

}

func TestFuzzCookieJsonPath(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "Cookie参数(JSON) 默认",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a={"number": 123,"boolean": true,"string": "123","json": {"a":"b"}}; zz=abcd

`,

				code: `.FuzzCookieJsonPath("a", "$.number", 999).FuzzCookieJsonPath("a", "$.boolean", false).FuzzCookieJsonPath("a", "$.string", "string").FuzzCookieJsonPath("a", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					url.QueryEscape(`"number":999`),
					url.QueryEscape(`"boolean":false`),
					url.QueryEscape(`"string":"string"`),
					url.QueryEscape(`"json":{"xx":123}`),
					`zz=abcd`,
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数(JSON) 默认 2",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=%7B%22number%22:%20123%2C%22boolean%22:%20true%2C%22string%22:%20%22123%22%2C%22json%22:%20%7B%22a%22:%22b%22%7D%7D

`,

				code: `.FuzzCookieJsonPath("a", "$.number", 999).FuzzCookieJsonPath("a", "$.boolean", false).FuzzCookieJsonPath("a", "$.string", "string").FuzzCookieJsonPath("a", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					url.QueryEscape(`"number":999`),
					url.QueryEscape(`"boolean":false`),
					url.QueryEscape(`"string":"string"`),
					url.QueryEscape(`"json":{"xx":123}`),
				},
				debug: true,
			},
		},

		{
			name: "Cookie参数(JSON) 禁止编码",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=%7B%22number%22:%20123%2C%22boolean%22:%20true%2C%22string%22:%20%22123%22%2C%22json%22:%20%7B%22a%22:%22b%22%7D%7D

`,

				code: `.FuzzCookieJsonPath("a", "$.number", 999).FuzzCookieJsonPath("a", "$.boolean", false).FuzzCookieJsonPath("a", "$.string", "string").FuzzCookieJsonPath("a", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`"number":999`,
					`"boolean":false`,
					`"string":"string"`,
					`"json":{"xx":123}`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
		{
			name: "Cookie参数(JSON) 友好显示",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a={"number": 123,"boolean": true,"string": "123","json": {"a":"b"}}

`,

				code: `.FuzzCookieJsonPath("a", "$.number", 999).FuzzCookieJsonPath("a", "$.boolean", false).FuzzCookieJsonPath("a", "$.string", "string").FuzzCookieJsonPath("a", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`{{urlescape(`,
					`"number":999`,
					`"boolean":false`,
					`"string":"string"`,
					`"json":{"xx":123}`,
					`)}}`,
				},
				debug:           true,
				friendlyDisplay: true,
			},
		},
		{
			name: "Cookie参数(JSON) 友好显示 && 禁止编码",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a={"number": 123,"boolean": true,"string": "123","json": {"a":"b"}}

`,

				code: `.FuzzCookieJsonPath("a", "$.number", 999).FuzzCookieJsonPath("a", "$.boolean", false).FuzzCookieJsonPath("a", "$.string", "string").FuzzCookieJsonPath("a", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`"number":999`,
					`"boolean":false`,
					`"string":"string"`,
					`"json":{"xx":123}`,
				},
				debug:           true,
				friendlyDisplay: true,
				disableEncode:   true,
			},
		},
		{
			name: "Cookie参数(JSON) 追加",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a={"number": 123,"boolean": true,"string": "123","json": {"a":"b"}}

`,

				code: `.FuzzCookieJsonPath("a", "$.number", 999).FuzzCookieJsonPath("a", "$.append", 123).FuzzCookieJsonPath("a", "$.append_string", "123")`,
				expectKeywordInOutputPacket: []string{
					`"number":999`,
					`"boolean":true`,
					`"string":"123"`,
					`"json":{"a":"b"}`,
					`"append":123`,
					`"append_string":"123"`,
				},
				debug:           true,
				friendlyDisplay: true,
				disableEncode:   true,
			},
		},
		{
			name: "Cookie参数(JSON) $[ bug",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=%7B%22distinct_id%22%3A%229b919660-ecfb-4cc1-b9b2-8986d8ae9251%22%2C%22first_id%22%3A%2218f57c14e841220-01c9a3b6ad31ef9-26001d51-1638720-18f57c14e851127%22%2C%22props%22%3A%7B%7D%2C%22%24device_id%22%3A%2218f57c14e841220-01c9a3b6ad31ef9-26001d51-1638720-18f57c14e851127%22%7D

`,
				// {"distinct_id":"9b919660-ecfb-4cc1-b9b2-8986d8ae9251","first_id":"18f57c14e841220-01c9a3b6ad31ef9-26001d51-1638720-18f57c14e851127","props":{},"$device_id":"18f57c14e841220-01c9a3b6ad31ef9-26001d51-1638720-18f57c14e851127"}
				code: `.FuzzCookieJsonPath("a", "$[\"$device_id\"]", 999)`,
				expectKeywordInOutputPacket: []string{
					`"$device_id":999`,
				},
				debug:           true,
				friendlyDisplay: true,
				disableEncode:   true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzCookieBase64JsonPath(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{ // base64 编码的 value 应当默认 disable auto encode
			name: "Cookie参数(Base64+JSON) 默认",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=eyJudW1iZXIiOjEyM30=; b=eyJib29sZWFuIjp0cnVlfQ==; c=eyJzdHJpbmciOiIxMjMifQ==; d=eyJqc29uIjp7Inh4IjoiYiJ9fQ==

`,
				//原始测试值 a={"number":123}; b={"boolean":true}; c={"string":"123"}; d={"json":{"xx":"b"}}
				code: `.FuzzCookieBase64JsonPath("a", "$.number", 999).FuzzCookieBase64JsonPath("b", "$.boolean", false).FuzzCookieBase64JsonPath("c", "$.string", "string").FuzzCookieBase64JsonPath("d", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`a=` + codec.EncodeBase64(`{"number":999}`),
					`b=` + codec.EncodeBase64(`{"boolean":false}`),
					`c=` + codec.EncodeBase64(`{"string":"string"}`),
					`d=` + codec.EncodeBase64(`{"json":{"xx":123}}`),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数(Base64+JSON) 追加",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=eyJudW1iZXIiOjEyM30=; b=eyJib29sZWFuIjp0cnVlfQ==; c=eyJzdHJpbmciOiIxMjMifQ==; d=eyJqc29uIjp7Inh4IjoiYiJ9fQ==

`,
				//原始测试值 a={"number":123}; b={"boolean":true}; c={"string":"123"}; d={"json":{"xx":"b"}}
				code: `.FuzzCookieBase64JsonPath("e", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`a=` + codec.EncodeBase64(`{"number":123}`),
					`b=` + codec.EncodeBase64(`{"boolean":true}`),
					`c=` + codec.EncodeBase64(`{"string":"123"}`),
					`d=` + codec.EncodeBase64(`{"json":{"xx":"b"}}`),
					`e=` + codec.EncodeBase64(`{"json":{"xx":123}}`),
				},
				debug: true,
			},
		},
		{
			name: "Cookie参数(Base64+JSON) 友好显示",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Cookie: a=eyJudW1iZXIiOjEyM30=; b=eyJib29sZWFuIjp0cnVlfQ==; c=eyJzdHJpbmciOiIxMjMifQ==; d=eyJqc29uIjp7Inh4IjoiYiJ9fQ==

`,
				//原始测试值 a={"number":123}; b={"boolean":true}; c={"string":"123"}; d={"json":{"xx":"b"}}
				code: `.FuzzCookieBase64JsonPath("a", "$.number", 999).FuzzCookieBase64JsonPath("b", "$.boolean", false).FuzzCookieBase64JsonPath("c", "$.string", "string").FuzzCookieBase64JsonPath("d", "$.json", {"xx":123}).FuzzCookieBase64JsonPath("e", "$.json", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`a={{base64({"number":999})}}`,
					`b={{base64({"boolean":false})}}`,
					`c={{base64({"string":"string"})}}`,
					`d={{base64({"json":{"xx":123}})}}`,
					`e={{base64({"json":{"xx":123}})}}`,
				},
				debug:           true,
				friendlyDisplay: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzPostJsonParams(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "JSON-Body参数",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

{"a": 222}
`,
				code: `.FuzzPostJsonParams("a", 123).FuzzPostJsonParams("b", 123).FuzzPostJsonParams("c", true).FuzzPostJsonParams("d", {"dd":123})`,
				expectKeywordInOutputPacket: []string{
					`{"a":123,"b":123,"c":true,"d":{"dd":123}}`,
				},
				//debug: true,
			},
		},
		{
			name: "JSON-Body参数 类型不匹配",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

{"number": 123,"boolean": true,"string": "123","json": {"a":"b"}}
`,
				code: `.FuzzPostJsonParams("number", true).FuzzPostJsonParams("boolean", 123).FuzzPostJsonParams("string", {"a":"b"}).FuzzPostJsonParams("json", "aaaa")`,
				expectKeywordInOutputPacket: []string{
					`"number":true`,
					`"boolean":123`,
					`"json":"aaaa"`,
					`"string":{"a":"b"}`,
				},
				debug: true,
			},
		},
		{
			name: "JSON-Body参数 string type",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

{"bc": "222"}
`,
				code: `.FuzzPostJsonParams("bc", "123")`,
				expectKeywordInOutputPacket: []string{
					`{"bc":"123"`,
				},
			},
		},
		{
			name: "JSON-Body参数 number type",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

{"bc": 123}
`,
				code: `.FuzzPostJsonParams("bc", 345)`,
				expectKeywordInOutputPacket: []string{
					`{"bc":345}`,
				},
			},
		},
		{
			name: "JSON-Body参数 boolean type",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

{"bc": false}
`,
				code: `.FuzzPostJsonParams("bc", true)`,
				expectKeywordInOutputPacket: []string{
					`{"bc":true}`,
				},
			},
		},
		{
			name: "JSON-Body参数 json type",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

{"bc": {"c":"d"}}
`,
				code: `.FuzzPostJsonParams("bc", {"zz":123})`,
				expectKeywordInOutputPacket: []string{
					`{"bc":{"zz":123}}`,
				},
			},
		},
		{
			name: "JSON-Body参数 json type",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com

[{"id": 1},{"id": 2}]
`,
				code: `.FuzzPostJsonParams("[0]", {"id":111})`,
				expectKeywordInOutputPacket: []string{
					`[{"id":111},{"id":2}]`,
				},
			},
		},
		{
			name: "JSON-Body参数 fastjson case",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/json;charset=UTF-8

{"Mv":{"@type":"java.lang.Class","val":"com.sun.rowset.JdbcRowSetImpl"}}`,
				code: `.FuzzPostJsonParams("$.Mv[\"@type\"]", "abcdefg")`, // json path 应当这样也行 $.Mv['@type']
				expectKeywordInOutputPacket: []string{
					`{"Mv":{"@type":"abcdefg"`,
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzPostParams(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "POST参数 默认",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==
`,
				code: `.FuzzPostParams("b", "%25%25").FuzzPostParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA%3D%3D",
					"b=%2525%2525",
					"c=%24",
				},
				debug: true,
			},
		},
		{
			name: "POST参数 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==
`,
				code: `.FuzzPostParams("b", "%25%25").FuzzPostParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a={{urlescape(MTIzNA==\n)}}",
					"b={{urlescape(%25%25)}}",
					"c={{urlescape($)}}",
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "POST参数 友好显示2",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==`,
				code: `.FuzzPostParams("b", "%25%25").FuzzPostParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a={{urlescape(MTIzNA==)}}&b={{urlescape(%25%25)}}&c={{urlescape($)}}",
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "POST参数 禁止指定参数自动编码",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==`,
				code: `.FuzzPostParams("b", "%25%25").FuzzPostParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA==",
					"b=%25%25",
					"c=$",
				},
				disableEncode: true,
				debug:         true,
			},
		},
		{
			name: "POST参数 禁止编码 & 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==`,
				code: `.FuzzPostParams("b", "%25%25")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA==", "b=%25%25",
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "POST参数 禁止编码 & 友好显示 2",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==`,
				code: `.FuzzPostParams("b", "%25").FuzzPostParams("c", "$")`,
				expectKeywordInOutputPacket: []string{
					"a=MTIzNA==",
					"b=%25",
					"c=$",
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "POST参数 友好显示 2",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==&b=2`,
				code: `.FuzzPostParams("b", "12")`,
				expectKeywordInOutputPacket: []string{
					"a={{urlescape(MTIzNA==)}}", "b=12",
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST参数 Raw",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=b
`,
				code: `.FuzzPostRaw("ccccccccccccccc")`,
				expectKeywordInOutputPacket: []string{
					"\r\n\r\nccccccccccccccc",
				},
			},
		},
		{
			name: "POST参数 Packet Num",
			base: base{
				inputPacket: `POST /?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzPostRaw("{{int(1-3)}}")`,
				expectKeywordInOutputPacket: []string{
					"\r\n\r\n1",
				},
				expectPacketNum: 3,
			},
		},
	}

	for _, tc := range tests {
		if tc.name != "POST参数 默认" {
			continue
		}
		t.Run(tc.name, testCaseCheck(tc.base))

	}
}

var raw = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
  <title>W3Schools Home Page</title>
  <link>https://www.w3schools.com</link>
  <description>Free web building tutorials</description>
  <item>
    <title>RSS Tutorial</title>
    <link>https://www.w3schools.com/xml/xml_rss.asp</link>
    <description>New RSS tutorial on W3Schools</description>
  </item>
  <item>
    <title>XML Tutorial</title>
    <link>https://www.w3schools.com/xml</link>
    <description>New XML tutorial on W3Schools</description>
  </item>
</channel>
</rss>`

func TestFuzzPostXMLParams(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "POST参数(XML) 默认",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

` + raw,
				code: `.FuzzPostXMLParams("//channel/title", "123").FuzzPostXMLParams("//channel/link", "https://yaklang.com")`,
				expectKeywordInOutputPacket: []string{
					`<channel><title>123</title><link>https://yaklang.com</link>`,
				},
			},
		},
		{
			name: "POST参数(XML) 默认2",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

` + raw,
				code: `.FuzzPostXMLParams("rss", "123")`,
				expectKeywordInOutputPacket: []string{
					`<rss version="2.0">123</rss>`,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}

}

func TestFuzzPostBase64Params(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "POST参数(Base64) 默认",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==&b=2`,
				code: `.FuzzPostBase64Params("a", 99999).FuzzPostBase64Params("c", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a=OTk5OTk%3D`,
					`b=2`,
					`c=OTk5OTk%3D`,
				},
			},
		},
		{
			name: "POST参数(Base64) 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==&b=2`,
				code: `.FuzzPostBase64Params("a", 99999).FuzzPostBase64Params("c", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({{base64(99999)}})}}&b=2&c={{urlescape({{base64(99999)}})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST参数(Base64) 禁止编码",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==&b=2`,
				code: `.FuzzPostBase64Params("a", 99999).FuzzPostBase64Params("c", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a=OTk5OTk=&b=2&c=OTk5OTk=`,
				},
				disableEncode: true,
				debug:         true,
			},
		},
		{
			name: "POST参数(Base64) 禁止编码 & 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==&b=2`,
				code: `.FuzzPostBase64Params("a", 99999).FuzzPostBase64Params("c", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a={{base64(99999)}}`,
					`b=2`,
					`c={{base64(99999)}}`,
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "POST参数(Base64) 禁止编码 & 友好显示 & Packet Num",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=MTIzNA==&b=2`,
				code: `.FuzzPostBase64Params("a", "{{int(1-3)}}")`,
				expectKeywordInOutputPacket: []string{
					`a={{base64(1)}}`,
					`b=2`,
				},
				expectPacketNum: 3,
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzPostJsonPathParams(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "POST 参数(JSON) 友好显示 类型不匹配",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": 123}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":"string"})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST 参数(JSON) string type 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": "123"}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", "99999")`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":"99999"})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST 参数(JSON) number type 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": 123}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", 99999)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":99999})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST 参数(JSON) boolean type 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": false}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", true)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":true})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST 参数(JSON) json type 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": {"c":"d"}}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", {"zz":123})`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":{"zz":123}})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST 参数(JSON) null type 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": null}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", 123)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":123})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST 参数(JSON) null type 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": 123}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", nil)`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape({"abc":null})}}`,
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "POST 参数(JSON) array type 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=[{"id": 1},{"id": 2}]`,
				code: `.FuzzPostJsonPathParams("a", "$.[0]", {"id":111})`,
				expectKeywordInOutputPacket: []string{
					`a={{urlescape([{"id":111},{"id":2}])}}`,
				},
				friendlyDisplay: true,
				debug:           true,
			},
		},
		{
			name: "POST 参数(JSON) 默认显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": 123}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a=%7B%22abc%22%3A%22string%22%7D`, // {"abc":"string"}
				},
			},
		},
		{
			name: "POST 参数(JSON) 禁止编码",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": 123}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a={"abc":"string"}`,
				},
				disableEncode: true,
			},
		},
		{
			name: "POST 参数(JSON) 禁止编码 & 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": 123}`,
				code: `.FuzzPostJsonPathParams("a", "$.abc", "string")`,
				expectKeywordInOutputPacket: []string{
					`a={"abc":"string"}`,
				},
				disableEncode:   true,
				friendlyDisplay: true,
			},
		},
		{
			name: "POST 参数(JSON) 禁止编码 & 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a={"abc": 123}`,
				code: `.FuzzPostJsonPathParams("a", "$.d", "dd").FuzzPostJsonPathParams("a", "$.e", 123).FuzzPostJsonPathParams("a", "$.f", {"xx":123})`,
				expectKeywordInOutputPacket: []string{
					`a={"abc":123,"d":"dd","e":123,"f":{"xx":123}}`,
				},
				disableEncode: true,
				//debug:         true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzPostBase64JsonPath(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "POST参数(Base64+JSON) 默认",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=ab&c=eyJkZCI6MTI1fQ%3D%3D`,
				code: `.FuzzPostBase64JsonPath("c", "$.dd", 1234)`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c=` + codec.QueryEscape(codec.EncodeBase64(`{"dd":1234}`)),
				},
			},
		},
		{
			name: "POST参数(Base64+JSON) 禁止编码",
			base: base{
				inputPacket: `POST /acc.t1 HTTP/1.1
Host: www.baidu.com

a=ab&c=eyJkZCI6MTI1fQ%3D%3D`,
				code: `.FuzzPostBase64JsonPath("c", "$.dd", 1234)`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c=` + codec.EncodeBase64(`{"dd":1234}`),
				},
				disableEncode: true,
				debug:         true,
			},
		},
		{
			name: "POST参数(Base64+JSON) 友好显示",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.baidu.com

a=ab&c=eyJkZCI6MTI1fQ%3D%3D`,
				code: `.FuzzPostBase64JsonPath("c", "$.dd", 1234)`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c={{urlescape({{base64({"dd":1234})}})}}`,
				},
				friendlyDisplay: true,
			},
		},
		{
			name: "POST参数(Base64+JSON) 友好显示 2",
			base: base{
				inputPacket: `POST /acc.t1 HTTP/1.1
Host: www.baidu.com

a=ab&c=W3siaWQiOiAxfSx7ImlkIjogMn1d`,
				code: `.FuzzPostBase64JsonPath("c", "$.[0]", {"xx":"bb"})`,
				expectKeywordInOutputPacket: []string{
					`a=ab`,
					`c={{urlescape({{base64([{"xx":"bb"},{"id":2}])}})}}`,
				},
				friendlyDisplay: true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestOOM(t *testing.T) {
	t.SkipNow()
	list := make([]string, 1<<21)

	for i := range list {
		list[i] = fmt.Sprintf("Value %d", i)
	}

	fmt.Println("list len:", len(list))

	jsonData, err := json.Marshal(list)
	if err != nil {
		t.FailNow()
	}
	dumpfile, err := os.CreateTemp("", "example")
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(dumpfile.Name()) // clean up

	if _, err := dumpfile.Write(jsonData); err != nil {
		t.FailNow()

	}

	prefix := `POST / HTTP/1.1
Host: www.baidu.com

`
	run := func() {
		content, err := os.ReadFile(dumpfile.Name())
		if err != nil {
			t.FailNow()
		}
		request, err := mutate.NewFuzzHTTPRequest(append([]byte(prefix), content...))
		if err != nil {
			t.FailNow()
		}
		params := request.GetPostJsonParams()
		for _, param := range params {
			//fmt.Println(param)
			_ = param
		}
	}
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	start := time.Now()
	run()
	elapsed := time.Since(start)

	runtime.ReadMemStats(&m2)
	fmt.Println("Time elapsed:", elapsed)
	fmt.Println("Alloc:", (m2.Alloc-m1.Alloc)/1024/1024, "m")
	fmt.Println("TotalAlloc:", (m2.TotalAlloc-m1.TotalAlloc)/1024/1024, "m")
	fmt.Println("HeapAlloc:", (m2.HeapAlloc-m1.HeapAlloc)/1024/1024, "m")
	fmt.Println("Mallocs:", (m2.Mallocs-m1.Mallocs)/1024/1024, "m")

}

func TestMultiple(t *testing.T) {
	tests := []struct {
		name string
		base base
	}{
		{
			name: "GET/POST参数 默认",
			base: base{
				inputPacket: `POST /zzz?a=1 HTTP/1.1
Host: www.example.com

c=1`,
				code: `.FuzzGetParams("a", "").FuzzPostParams("c", "")`,
				expectKeywordInOutputPacket: []string{
					"zzz?a=",
					"\r\nc=",
				},
				debug: true,
			},
		},
		{
			name: "GET/POST参数 禁止编码",
			base: base{
				inputPacket: `POST /zzz?a=1 HTTP/1.1
Host: www.example.com

c=1`,
				code: `.FuzzGetParams("a", "").FuzzPostParams("c", "")`,
				expectKeywordInOutputPacket: []string{
					"zzz?a=",
					"\r\nc=",
				},
				disableEncode: true,
				debug:         true,
			},
		},
		{
			name: "GET/POST/PATH/APPEND 禁止编码",
			base: base{
				inputPacket: `POST /zzz?a=1 HTTP/1.1
Host: www.example.com

c=1`,
				code: `.FuzzGetParams("a", "").FuzzPostParams("c", "").FuzzPath("/1").FuzzPathAppend("/2/").FuzzPathAppend("/3")`,
				expectKeywordInOutputPacket: []string{
					"/1/2//3?a=",
					"\r\nc=",
				},
				disableEncode: true,
				debug:         true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))

	}
}
