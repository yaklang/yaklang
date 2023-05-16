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
			InputPacket: `GET / HTTP/1.1
Host: www.baidu.com
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzFormEncoded(`Key`, 123)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", `Content-Disposition: form-data; name="Key"` + "\r\n\r\n123\r\n--"},
		},
		{
			InputPacket: `GET /?a={"abc": 123} HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetJsonPathParams(`a`, `$.abc`, `a123aaa1`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "%7B%22abc%22%3A%22a123aaa1%22%7D"},
		},
		{
			InputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParams(`a`, `$.abc`).FuzzGetParams(`ccc`, `12`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "a=%24.abc", "ccc=12"},
		},
		{
			InputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParams(`a`, `$.abc`).FuzzGetParams(`ccc`, `12`).FuzzGetParamsRaw(`ccccccccccccccc`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "/?ccccccccccccccc"},
		},
		{
			InputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /?ccccccccccccccc"},
		},
		{
			InputPacket: `GET /?a=ab HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc"},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`/12`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t1/12?ccccccccccccccc"},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc"},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

{"bc": 222}
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonParams(`bc`, 123)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc", `{"bc":123}`},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

{"bc": 222}
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonParams(`bc`, 123).FuzzPostJsonParams(`ddddddd`, `dd1`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc", `"bc":123`, `"ddddddd":"dd1"`},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonParams(`bc`, 123).FuzzPostJsonParams(`ddddddd`, `dd1`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc", `"bc":123`, `"ddddddd":"dd1"`},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonPathParams(`c`, `$.abc.c.d`, 123)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc", `%7B%22abc%22%3A%7B%22c%22%3A%7B%22d%22%3A123%7D%7D%7D`},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonPathParams(`c`, `$.abc.c.d`, 123).FuzzPostParams(`d`, `abc`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc", `%7B%22abc%22%3A%7B%22c%22%3A%7B%22d%22%3A123%7D%7D%7D`, `d=abc`},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
			Code:                        ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzPath(`/acc.t1`).FuzzPathAppend(`12`).FuzzPostJsonPathParams(`c`, `$.abc.c.d`, 123).FuzzPostParams(`d`, `abc`).FuzzPostRaw(`dhjkasdhjkasjkhdihasdhiouwaioheriohqweiohqweiohqiwhet--=-=-=-=-=-`)",
			ExpectKeywordInOutputPacket: []string{"ABC: CCC\r\n", "XXX /acc.t112?ccccccccccccccc", `dhjkasdhjkasjkhdihasdhiouwaioheriohqweiohqweiohqiwhet--=-=-=-=-=-`},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
			Code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFile(`ccc`, `abc.php`, `<?=1+1?>`)",
			ExpectKeywordInOutputPacket: []string{
				"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
				"; filename=\"abc.php\"", `<?=1+1?>` + "\r\n--",
				`multipart/form-data; boundary=-`,
			},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
			Code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFileName(`ccc`, `abc.php`)",
			ExpectKeywordInOutputPacket: []string{
				"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
				"; filename=\"abc.php\"",
				`multipart/form-data; boundary=-`,
			},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
			Code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFileName(`ccc`, `abc.php`).FuzzUploadKVPair(`cccddd`, `abccc.123.ph`).FuzzUploadFile(`your-filename`, 'php.pp12.txt', `adfkdsjklasjkldjklasdfjklasdf`)",
			ExpectKeywordInOutputPacket: []string{
				"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
				"; filename=\"abc.php\"",
				`multipart/form-data; boundary=-`,
				`name="your-filename"; filename="php.pp12.txt"`,
				`adfkdsjklasjkldjklasdfjklasdf` + "\r\n--",
				"name=\"cccddd\"\r\n\r\nabccc.123.ph\r\n--",
			},
		},
		{
			InputPacket: `GET /acc.t1?a=ab HTTP/1.1
Host: www.baidu.com
Cookie: abc={"ccc":2311}

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
			Code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetParamsRaw(`ccccccccccccccc`).FuzzMethod(`XXX`).FuzzUploadFileName(`ccc`, `abc.php`).FuzzUploadKVPair(`cccddd`, `abccc.123.ph`).FuzzUploadFile(`your-filename`, 'php.pp12.txt', `adfkdsjklasjkldjklasdfjklasdf`).FuzzCookieJsonPath(`abc`, `$.ccc`, `zk123`)",
			ExpectKeywordInOutputPacket: []string{
				"ABC: CCC\r\n", "XXX /acc.t1?ccccccccccccccc",
				"; filename=\"abc.php\"",
				`multipart/form-data; boundary=-`,
				`name="your-filename"; filename="php.pp12.txt"`,
				`adfkdsjklasjkldjklasdfjklasdf` + "\r\n--",
				"name=\"cccddd\"\r\n\r\nabccc.123.ph\r\n--",
				"zk123", `%7B%22ccc%22%3A%22zk123%22%7D`,
			},
		},
		{
			InputPacket: `GET /acc.t1?a=ab&&c=eyJkZCI6MTI1fQ%3D%3D HTTP/1.1
Host: www.baidu.com
Cookie: abc={"ccc":2311}

c={"abc":{"c":{"d":true}}}&&d=1234444
`,
			Code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzGetBase64JsonPath(`c`, `$.dd`, `ddda`)",
			ExpectKeywordInOutputPacket: []string{
				"ABC: CCC\r\n",
				"a=ab", "c=ey",
				"eyJkZCI6ImRkZGEifQ%3D%3D",
				"c=eyJkZCI6ImRkZGEifQ%3D%3D",
			},
		},
		{
			InputPacket: `GET /acc.t1?a=ab&&c=eyJkZCI6MTI1fQ%3D%3D HTTP/1.1
Host: www.baidu.com
Cookie: abc={"ccc":2311}

c=eyJkZCI6MTI1fQ%3D%3D&&d=1234444
`,
			Code: ".FuzzHTTPHeader(\"ABC\", \"CCC\").FuzzPostBase64JsonPath(`c`, `$.dd`, `ddda`)",
			ExpectKeywordInOutputPacket: []string{
				"ABC: CCC\r\n",
				"a=ab", "c=ey",
				"eyJkZCI6ImRkZGEifQ%3D%3D",
				"c=eyJkZCI6ImRkZGEifQ%3D%3D",
			},
			Debug: true,
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
			fmt.Println("CHECK FAILED CODE: ")
			fmt.Println(data.Code)
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
