package mutate_tests

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"strings"
	"testing"
)

func init() {
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()

	_ = yaklang.New()
}

type BaseCase struct {
	InputPacket                 string
	Code                        string
	ExpectKeywordInOutputPacket []string
	ExpectRegexpInOutputPacket  []string
	Debug                       bool
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

func TestYaklangFuzzHTTPRequestBaseCase(t *testing.T) {
	_ = yak.NewScriptEngine(1)

	total := []*BaseCase{
		{
			InputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\")",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n"},
		},
		{
			InputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzCookie(`foo`, `bar11`).FuzzCookie(`c`, `123`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "foo=bar11", `c=123`},
		},
		{
			InputPacket: `GET / HTTP/1.1
Host: www.baidu.com`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzCookieRaw(`CAasd9y812589yasdjkladsf`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", `CAasd9y812589yasdjkladsf` + "\r\n"},
		},
		{
			InputPacket: `GET / HTTP/1.1
Host: www.baidu.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzFormEncoded(`Key`, 123)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", `Content-Disposition: form-data; name="Key"` + "\r\n\r\n123\r\n--"},
		},
		{
			Debug: true,
			InputPacket: `GET / HTTP/1.1
Host: www.baidu.com
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzFormEncoded(`Key`, 123)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", `Content-Disposition: form-data; name="Key"` + "\r\n\r\n123\r\n--"},
		},
	}

	debugCases := funk.Filter(total, func(i *BaseCase) bool {
		return i.Debug
	}).([]*BaseCase)
	ordinaryCases := funk.Filter(total, func(i *BaseCase) bool {
		return !i.Debug
	}).([]*BaseCase)

	test := assert.New(t)
	handle := func(data *BaseCase) {
		ctx := context.Background()
		engine := yaklang.New().(*antlr4yak.Engine)
		engine.SetVar("request", data.InputPacket)
		engine.SetVar("keywords", data.ExpectKeywordInOutputPacket)
		engine.SetVar("regexps", data.ExpectRegexpInOutputPacket)
		engine.SetVar("debug", data.Debug)

		if data.Code != "" {
			data.Code = "." + strings.TrimLeft(data.Code, ".")
		}
		initCode := `result = fuzz.HTTPRequest(request)~` + data.Code
		if data.Debug {
			fmt.Println("----------------OP CODE-----------------")
			fmt.Println(initCode)
			fmt.Println("----------------------------------------")
		}
		err := engine.EvalInline(ctx, initCode)
		if err != nil {
			test.Fail("eval code failed: %s", err)
			return
		}

		if data.Debug {
			fmt.Println("----------------KEYWORD-----------------")
			engine.EvalInline(ctx, "dump(keywords)")
			fmt.Println("----------------REGEXPS-----------------")
			engine.EvalInline(ctx, "dump(regexps)")
			fmt.Println()
		}

		err = engine.EvalInline(context.Background(), `raw = result.GetFirstFuzzHTTPRequest()~.GetBytes()
if debug { println(string(raw)) }
check = false
if str.MatchAllOfSubString(raw, keywords...) || str.MatchAllOfRegexp(raw, regexps...){
    check = true
}`)
		if err != nil {
			test.Fail("eval code failed: %s", err)
			return
		}

		checked, ok := engine.GetVar("check")
		if !ok {
			test.Fail("getvar[check] failed")
		}
		if !checked.(bool) {
			println(string(data.InputPacket))
			test.FailNow("check failed")
		}
	}

	for _, c := range debugCases {
		handle(c)
	}
	for _, c := range ordinaryCases {
		handle(c)
	}

}
