package mutate_tests

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"strings"
	"testing"
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

		engine.SetVar("request", data.inputPacket)
		engine.SetVar("keywords", data.expectKeywordInOutputPacket)
		engine.SetVar("regexps", data.expectRegexpInOutputPacket)
		engine.SetVar("debug", data.debug)

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
					"a=MTIzNA%3D%3D",
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
					"a={{urlescape(MTIzNA==)}}", "b=%25%25",
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
					"a={{urlescape(MTIzNA==)}}",
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
	}
	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestFuzzGetJsonPathParams(t *testing.T) {
	// 现在的处理逻辑是先解析json，根据json path获取值
	// 根据原值的类型进行 fuzz
	// 如果原值与 Fuzz 值不匹配，默认使用字符串类型
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
			name: "URL路径 ",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

123456
`,
				code: `.FuzzPath("a")`,
				expectKeywordInOutputPacket: []string{
					`/a?a=ab`,
				},
				debug:         true,
				disableEncode: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, testCaseCheck(tc.base))
	}
}

func TestYaklangFuzzHTTPRequestBaseCase(t *testing.T) {

	tests := []struct {
		name string
		base base
	}{
		{
			name: "Fuzz HTTP Header",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\")",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n"},
			},
		},
		{
			name: "Fuzz HTTP Header and Cookie",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzCookie(`foo`, `bar11`).FuzzCookie(`c`, `123`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "foo=bar11", `c=123`},
			},
		},

		{
			name: "Fuzz HTTP Header and Cookie Raw",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzCookieRaw(`CAasd9y812589yasdjkladsf`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", `CAasd9y812589yasdjkladsf` + "\r\n"},
			},
		},
		{
			name: "Fuzz HTTP Header and Form Encoded",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW
`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzFormEncoded(`Key`, 123)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", `Content-Disposition: form-data; name="Key"` + "\r\n\r\n123\r\n--"},
			},
		},
		{
			name: "Fuzz HTTP Header and Form Encoded Raw no Content-Type",
			base: base{
				inputPacket: `GET / HTTP/1.1
Host: www.baidu.com
`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzFormEncoded(`Key`, 123)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", `Content-Disposition: form-data; name="Key"` + "\r\n\r\n123\r\n--"},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Json Path Params",
			base: base{
				inputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetJsonPathParams(`a`, `$.abc`, `a123aaa1`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "%7B%22abc%22%3A%22a123aaa1%22%7D"},
			},
		},

		{
			name: "Fuzz HTTP Header and Get Params",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParams(`a`, `$.abc`).FuzzGetParams(`ccc`, `12`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "a=%24.abc", "ccc=12"},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Params and Get Params Raw",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParams(`a`, `$.abc`).FuzzGetParams(`ccc`, `12`).FuzzGetParamsRaw(`ccccccccccccccc`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "/?ccccccccccccccc"},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Params Raw and Fuzz Method",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /?ccccccccccccccc"},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Params Raw and Fuzz Method and Fuzz Path",
			base: base{
				inputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc"},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Params Raw and Fuzz Method and Fuzz Path and Fuzz Path Append",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`/12`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t1/12?ccccccccccccccc"},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Params Raw and Fuzz Method and Fuzz Path and Fuzz Path Append 2",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`)",
				expectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc"},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Params Raw and Fuzz Method and Fuzz Path and Fuzz Path Append and Fuzz Post Json Params",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

{"bc": 222}
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonParams(`bc`, 123)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n",
					"XXX /acc.t112?ccccccccccccccc",
					// 原始json中的空格会被保留
					`{"bc": 123}`,
				},
			},
		},
		{
			name: "Fuzz HTTP Header and Get Params Raw and Method and  Path and Path Append and Post Json Params 2",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

{"bc": 222}
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonParams(`bc`, 123).FuzzPostJsonParams(`ddddddd`, `dd1`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n",
					"XXX /acc.t112?ccccccccccccccc",
					`"bc": 123`, `"ddddddd":"dd1"`,
				},
				debug: true,
			},
		},
		{
			name: "Fuzz Post Json Params no Body",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonParams(`bc`, 123).FuzzPostJsonParams(`ddddddd`, `dd1`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n",
					"XXX /acc.t112?ccccccccccccccc",
					`"bc":123`,
					`"ddddddd":"dd1"`,
				},
				debug: true,
			},
		},
		{
			name: "Fuzz Post Json Path Params",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonPathParams(`c`, `abc.c.d`, false)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc",
					`c={{urlescape({"abc":{"c":{"d":false}}})}}`,
				},
				debug: true,
			},
		},
		{
			name: "Fuzz Multiple Post Json Path Params",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonPathParams(`c`, `$.abc.c.d`, 123).FuzzPostParams(`d`, `abc`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n",
					"XXX /acc.t112?ccccccccccccccc",
					`%7B%22abc%22%3A%7B%22c%22%3A%7B%22d%22%3A123%7D%7D%7D`,
					`d=abc`,
				},
			},
		},
		{
			name: "Fuzz Multiple Post Json Path Params and Post Raw",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonPathParams(`c`, `$.abc.c.d`, 123).FuzzPostParams(`d`, `abc`).FuzzPostRaw(`dhjkasdhjkasjkhdihasdhiouwaioheriohqweiohqweiohqiwhet--=-=-=-=-=-`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc",
					`dhjkasdhjkasjkhdihasdhiouwaioheriohqweiohqweiohqiwhet--=-=-=-=-=-`,
				},
			},
		},
		{
			name: "Fuzz Upload File",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFile(`ccc`, `abc.php`, `<?=1+1?>`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
					"; filename=\"abc.php\"", `<?=1+1?>` + "\r\n--",
					`multipart/form-data; boundary=-`,
				},
			},
		},
		{
			name: "Fuzz Upload File Name",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFileName(`ccc`, `abc.php`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
					"; filename=\"abc.php\"",
					`multipart/form-data; boundary=-`,
				},
			},
		},
		{
			name: "Fuzz Multiple Upload File",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFileName(`ccc`, `abc.php`).FuzzUploadKVPair(`cccddd`, `abccc.123.ph`).FuzzUploadFile(`your-filename`, 'php.pp12.txt', `adfkdsjklasjkldjklasdfjklasdf`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
					"; filename=\"abc.php\"",
					`multipart/form-data; boundary=-`,
					`name="your-filename"; filename="php.pp12.txt"`,
					`adfkdsjklasjkldjklasdfjklasdf` + "\r\n--",
					"name=\"cccddd\"\r\n\r\nabccc.123.ph\r\n--",
				},
			},
		},
		{
			name: "Fuzz Upload File Name with fuzztag",
			base: base{
				inputPacket: `POST / HTTP/1.1
Host: www.example.com
Content-Type: multipart/form-data; boundary=------------------------mElesrxgGfeRzfHJlyONsWWKKiqXIiVGVuaxYhpG
Content-Length: 245

--------------------------mElesrxgGfeRzfHJlyONsWWKKiqXIiVGVuaxYhpG
Content-Disposition: form-data; name="a"; filename="a.php"
Content-Type: application/octet-stream


--------------------------mElesrxgGfeRzfHJlyONsWWKKiqXIiVGVuaxYhpG--`,
				code: ".FuzzUploadFileName(\"a\",\"abc{{i(1-2)}}.php\")",
				expectKeywordInOutputPacket: []string{
					"name=\"a\"; filename=\"abc1.php\"",
				},
			},
		},
		{
			name: "Fuzz Multiple Params",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com
Cookie: abc={"ccc":2311}

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFileName(`ccc`, `abc.php`).FuzzUploadKVPair(`cccddd`, `abccc.123.ph`).FuzzUploadFile(`your-filename`, 'php.pp12.txt', `adfkdsjklasjkldjklasdfjklasdf`).FuzzCookieJsonPath(`abc`, `$.ccc`, `zk123`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
					"; filename=\"abc.php\"",
					`multipart/form-data; boundary=-`,
					`name="your-filename"; filename="php.pp12.txt"`,
					`adfkdsjklasjkldjklasdfjklasdf` + "\r\n--",
					"name=\"cccddd\"\r\n\r\nabccc.123.ph\r\n--",
					"zk123", `%7B%22ccc%22%3A%22zk123%22%7D`,
				},
			},
		},
		{
			name: "Fuzz Get Base64 Json Path",
			base: base{
				inputPacket: `GET /acc.t1?a=ab&&c=eyJkZCI6MTI1fQ%3D%3D HTTP/1.1
Host: www.baidu.com
Cookie: abc={"ccc":2311}

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetBase64JsonPath(`c`, `$.dd`, `ddda`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n",
					"a=ab", "c=ey",
					"eyJkZCI6ImRkZGEifQ%3D%3D",
					"c=eyJkZCI6ImRkZGEifQ%3D%3D",
				},
			},
		},
		{
			name: "Fuzz Post Base64 Json Path",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com
Cookie: abc={"ccc":2311}

c=eyJkZCI6MTI1fQ%3D%3D&&d=1234444
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzPostBase64JsonPath(`c`, `$.dd`, `ddda`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n",
					"a=ab", "c=ey",
					"eyJkZCI6ImRkZGEifQ%3D%3D",
					"c=eyJkZCI6ImRkZGEifQ%3D%3D",
				},
			},
		},
		{
			name: "Fuzz Cookie Base64 Json Path",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com
Cookie: c=eyJkZCI6MTI1fQ%3D%3D

d=1234444&&qa=1
`,
				code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzCookieBase64JsonPath(`c`, `$.dd`, `ddda`)",
				expectKeywordInOutputPacket: []string{
					"ABC: CCC\r\n",
					"a=ab", "Cookie: c=ey",
					"eyJkZCI6ImRkZGEifQ%3D%3D",
					"c=eyJkZCI6ImRkZGEifQ%3D%3D",
				},
			},
		},

		{
			name: "Fuzz Post Json Type Params",
			base: base{
				inputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

`,
				code: `.FuzzPostJsonParams("aaa", 123456789).FuzzPostJsonParams("bbb", "123456789").FuzzPostJsonParams("ccc",{"cd":"{{i(1-2)}}"})`,
				expectKeywordInOutputPacket: []string{
					`"aaa":123456789`,
					`"bbb":"123456789"`,
					`"ccc":{"cd":"1"}`,
				},
			},
		},
		{
			name: "临时",
			base: base{
				inputPacket: `GET /acc.t1 HTTP/1.1
Host: www.baidu.com

a={"key":1111111}
`,

				code: ".FuzzPostJsonPathParams(`a`, `$.key`, 2222)",
				expectKeywordInOutputPacket: []string{
					`GET /acc.t1?a={{base64({"key":2222})}}`,
				},
				debug: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			test := assert.New(t)
			ctx := context.Background()
			engine := yaklang.New()
			data := tc.base

			engine.SetVar("request", data.inputPacket)
			engine.SetVar("keywords", data.expectKeywordInOutputPacket)
			engine.SetVar("regexps", data.expectRegexpInOutputPacket)
			engine.SetVar("debug", data.debug)

			if data.code != "" {
				data.code = "." + strings.TrimLeft(data.code, ".")
			}
			initCode := `result = fuzz.HTTPRequest(request)~` + data.code
			if data.debug {
				fmt.Println("----------------OP CODE-----------------")
				fmt.Println(initCode)
				fmt.Println("----------------------------------------")
			}
			err := engine.EvalInline(ctx, initCode)
			test.NoError(err, "eval code should not fail")

			if data.debug {
				fmt.Println("----------------KEYWORD-----------------")
				engine.EvalInline(ctx, "dump(keywords)")
				fmt.Println("----------------REGEXPS-----------------")
				engine.EvalInline(ctx, "dump(regexps)")
				fmt.Println()
			}

			err = engine.EvalInline(context.Background(), `raw = result.GetFirstFuzzHTTPRequest()~.GetBytes()
if debug { println(string(raw)) }
check = str.MatchAllOfSubString(raw, keywords...) || str.MatchAllOfRegexp(raw, regexps...)`)
			test.NoError(err, "eval code should not fail")

			checked, ok := engine.GetVar("check")
			test.True(ok, "should get 'check' variable")
			test.True(checked.(bool), "check should be true")
		})
	}

}
