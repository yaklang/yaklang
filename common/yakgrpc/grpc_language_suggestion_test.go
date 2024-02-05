package yakgrpc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func GetSuggestion(local ypb.YakClient, typ, pluginType string, t *testing.T, code string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
	t.Log("========== get ", typ)
	ret, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   typ,
		YakScriptType: pluginType,
		YakScriptCode: code,
		Range:         Range,
	})
	log.Info(ret)
	if err != nil {
		t.Fatal(err)
	}
	// t.Log(ret)
	return ret
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionCompletion(t *testing.T) {
	local, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	getCompletion := func(t *testing.T, code string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
		return GetSuggestion(local, "completion", "yak", t, code, Range)
	}

	t.Run("check basic extern-lib completion", func(t *testing.T) {
		res := getCompletion(t, `
cli.
	`, &ypb.Range{
			Code:        "",
			StartLine:   2,
			StartColumn: 4,
			EndLine:     2,
			EndColumn:   4,
		})
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("code `cli.` should get completion but not")
		}
	})

	t.Run("check extern struct completion", func(t *testing.T) {
		res := getCompletion(t, `
prog = ssa.Parse("")~
prog.
		`, &ypb.Range{
			Code:        "prog.",
			StartLine:   3,
			StartColumn: 0,
			EndLine:     3,
			EndColumn:   6,
		})
		got := lo.Map(res.SuggestionMessage, func(item *ypb.SuggestionDescription, _ int) string {
			return item.Label
		})
		log.Info("got: ", got)
		want := []string{"Program", "Ref"}
		if !utils.StringSliceContainsAll(got, want...) {
			t.Fatalf("want %v, but got %v", want, got)
		}
	})

	t.Run("check anyonmous field struct completion", func(t *testing.T) {
		res := getCompletion(t, `
rsp, err = http.Request("GET", "https://baidu.com")
rsp.
		`, &ypb.Range{
			Code:        "rsp.",
			StartLine:   3,
			StartColumn: 0,
			EndLine:     3,
			EndColumn:   4,
		})
		got := lo.Map(res.SuggestionMessage, func(item *ypb.SuggestionDescription, _ int) string {
			return item.Label
		})
		log.Info("got: ", got)
		want := []string{"Response", "Body", "Status", "Data"}
		if !utils.StringSliceContainsAll(got, want...) {
			t.Fatalf("want %v, but got %v", want, got)
		}
	})
}

var local ypb.YakClient = nil

func CheckHover(t *testing.T) func(t *testing.T, code, typ string, Range *ypb.Range, want string, subStr ...bool) {
	if local == nil {
		var err error
		local, err = NewLocalClient()
		if err != nil {
			t.Fatal(err)
		}
	}

	getHover := func(t *testing.T, code, typ string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
		return GetSuggestion(local, "hover", typ, t, code, Range)
	}
	check := func(t *testing.T, code, typ string, Range *ypb.Range, want string, sub ...bool) {
		subStr := false
		for _, v := range sub {
			if v {
				subStr = true
				break
			}
		}

		req := getHover(t, code, typ, Range)
		log.Info(req.SuggestionMessage)
		if len(req.SuggestionMessage) != 1 {
			t.Fatal("should get 1 suggestion")
		}
		got := req.SuggestionMessage[0].Label
		if subStr {
			if !strings.Contains(got, want) {
				t.Fatalf("want %s, but get %s", want, got)
			}
		} else {
			if got != want {
				t.Fatalf("want %s, but get %s", want, got)
			}
		}
	}
	return check
}

func CheckSignature(t *testing.T) func(t *testing.T, code, typ string, Range *ypb.Range, wantLabel string, wantDesc string, subStr ...bool) {
	if local == nil {
		var err error
		local, err = NewLocalClient()
		if err != nil {
			t.Fatal(err)
		}
	}

	getHover := func(t *testing.T, code, typ string, Range *ypb.Range) *ypb.YaklangLanguageSuggestionResponse {
		return GetSuggestion(local, "signature", typ, t, code, Range)
	}
	check := func(t *testing.T, code, typ string, Range *ypb.Range, wantLabel string, wantDesc string, sub ...bool) {
		subStr := false
		for _, v := range sub {
			if v {
				subStr = true
				break
			}
		}

		req := getHover(t, code, typ, Range)
		log.Info(req.SuggestionMessage)
		if len(req.SuggestionMessage) != 1 {
			t.Fatal("should get 1 suggestion")
		}
		got := req.SuggestionMessage[0].Label
		if subStr {
			if !strings.Contains(got, wantLabel) {
				t.Fatalf("want %s, but get %s", wantLabel, got)
			}
		} else {
			if got != wantLabel {
				t.Fatalf("want %s, but get %s", wantLabel, got)
			}
		}
		got = req.SuggestionMessage[0].Description
		if subStr {
			if !strings.Contains(got, wantDesc) {
				t.Fatalf("want %s, but get %s", wantDesc, got)
			}
		} else {
			if got != wantDesc {
				t.Fatalf("want %s, but get %s", wantDesc, got)
			}
		}
	}
	return check
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_Basic(t *testing.T) {
	check := CheckHover(t)

	t.Run("check basic hover", func(t *testing.T) {
		code := `
				 a = 1
				 b = 1.1
				 c = "asd"
				 d = b"asd"; d2 = []byte("asd")
				 e = {"a": 1}
				 f = [1, 2, 3]
				 g = make(chan int)
				 `
		check(t, code, "yak", &ypb.Range{
			Code:        "a",
			StartLine:   2,
			StartColumn: 5,
			EndLine:     2,
			EndColumn:   6,
		}, "```go\na number\n```")
		check(t, code, "yak", &ypb.Range{
			Code:        "b",
			StartLine:   3,
			StartColumn: 5,
			EndLine:     3,
			EndColumn:   6,
		}, "```go\nb number\n```")
		check(t, code, "yak", &ypb.Range{
			Code:        "c",
			StartLine:   4,
			StartColumn: 5,
			EndLine:     4,
			EndColumn:   6,
		}, "```go\nc string\n```")
		check(t, code, "yak", &ypb.Range{
			Code:        "d",
			StartLine:   5,
			StartColumn: 5,
			EndLine:     5,
			EndColumn:   6,
		}, "```go\nd []byte\n```")
		check(t, code, "yak", &ypb.Range{
			Code:        "d2",
			StartLine:   5,
			StartColumn: 11,
			EndLine:     5,
			EndColumn:   13,
		}, "```go\nd2 []byte\n```")
		check(t, code, "yak", &ypb.Range{
			Code:        "e",
			StartLine:   6,
			StartColumn: 5,
			EndLine:     6,
			EndColumn:   6,
		}, "```go\ne map[any]any\n```")
		check(t, code, "yak", &ypb.Range{
			Code:        "f",
			StartLine:   7,
			StartColumn: 5,
			EndLine:     7,
			EndColumn:   6,
		}, "```go\nf []any\n```")
		check(t, code, "yak", &ypb.Range{
			Code:        "g",
			StartLine:   8,
			StartColumn: 5,
			EndLine:     8,
			EndColumn:   6,
		}, "```go\ng chan number\n```")
	})
	t.Run("check mitm hover argument", func(t *testing.T) {
		check(t, `
		hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
			responseBytes, _ = codec.StrconvUnquote(flow.Response)
			a = flow.BeforeSave() //error
		}
		`,
			"mitm",
			&ypb.Range{
				Code:        "modify",
				StartLine:   2,
				StartColumn: 56,
				EndLine:     2,
				EndColumn:   62,
			},
			"```go\nfunc modify(r1 yakit.HTTPFlow) null\n```",
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_ExternLib(t *testing.T) {
	check := CheckHover(t)
	codeTemplate := `%s {
prog  = ssa.Parse(
    "", 
    ssa.withLanguage(
        ssa.Javascript
    )
)~
prog.Packages
}`

	type CheckItem struct {
		name      string
		want      string
		Range     *ypb.Range
		subString bool
	}

	data := []CheckItem{
		{
			name: "extern lib",
			want: getExternLibDesc("ssa", "any"),
			Range: &ypb.Range{
				Code:        "ssa",
				StartLine:   2,
				StartColumn: 8,
				EndLine:     2,
				EndColumn:   11,
			},
		},
		{
			name: "extern lib method",
			want: getFuncDeclDesc(getFuncDeclByName("ssa.Parse"), "Parse"),
			Range: &ypb.Range{
				Code:        "ssa.Parse",
				StartLine:   2,
				StartColumn: 8,
				EndLine:     2,
				EndColumn:   17,
			},
		},
		{
			name: "extern lib instance",
			want: getConstInstanceDesc(getInstanceByName("ssa.Javascript")),
			Range: &ypb.Range{
				Code:        "ssa.Javascript",
				StartLine:   5,
				StartColumn: 8,
				EndLine:     5,
				EndColumn:   22,
			},
		},
		{
			name: "extern lib method return",
			want: `func (Program) Ref(name string) Value`,
			Range: &ypb.Range{
				Code:        "prog",
				StartLine:   2,
				StartColumn: 0,
				EndLine:     2,
				EndColumn:   4,
			},
			subString: true,
		},
	}

	test := map[string]string{
		"normal":     "",
		"in loop":    "for a ",
		"in closure": "f = () => ",
	}

	for testName, prefix := range test {
		code := fmt.Sprintf(codeTemplate, prefix)
		for _, item := range data {
			t.Run(fmt.Sprintf("test %s %s", item.name, testName), func(t *testing.T) {
				check(t, code, "yak", item.Range, item.want, item.subString)
			})
		}
	}
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_StructMemberAndMethod(t *testing.T) {
	check := CheckHover(t)
	code := `rsp, err = http.Request("GET", "https://baidu.com")
rsp.Status
rsp.Data()`
	t.Run("check member hover", func(t *testing.T) {
		ssaRange := &ypb.Range{
			Code:        "rsp.Status",
			StartLine:   2,
			StartColumn: 0,
			EndLine:     2,
			EndColumn:   10,
		}
		want := "```go\n" + `field Status string` + "\n```"
		check(t, code, "yak", ssaRange, want)
	})

	t.Run("check method hover", func(t *testing.T) {
		ssaParseRange := &ypb.Range{
			Code:        "rsp.Data",
			StartLine:   3,
			StartColumn: 0,
			EndLine:     3,
			EndColumn:   8,
		}
		// 标准库函数
		want := "```go\n" + `func Data() string` + "\n```"
		check(t, code, "yak", ssaParseRange, want)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionSignature(t *testing.T) {
	check := CheckSignature(t)
	code := `a = func(b, c...) {}
a()
poc.HTTP()
`

	t.Run("check standard library function signature", func(t *testing.T) {
		ssaRange := &ypb.Range{
			Code:        "poc.HTTP",
			StartLine:   3,
			StartColumn: 0,
			EndLine:     3,
			EndColumn:   8,
		}
		wantLabel := "HTTP(i any, opts ...PocConfigOption) (rsp []byte, req []byte, err error)"
		wantDesc := "HTTP 发送请求并且返回原始响应报文，原始请求报文以及错误，它的第一个参数可以接收[]byte, string, http.Request结构体，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置超时时间，或者修改请求报文等\n\nExample:\n```\npoc.HTTP(\"GET / HTTP/1.1\\r\\nHost: www.yaklang.com\\r\\n\\r\\n\", poc.https(true), poc.replaceHeader(\"AAA\", \"BBB\")) // yaklang.com发送一个基于HTTPS协议的GET请求，并且添加一个请求头AAA，它的值为BBB\n```\n"
		check(t, code, "yak", ssaRange, wantLabel, wantDesc)
	})
	t.Run("check user function signature", func(t *testing.T) {
		ssaRange := &ypb.Range{
			Code:        "a",
			StartLine:   2,
			StartColumn: 0,
			EndLine:     2,
			EndColumn:   1,
		}
		wantLabel := "func a(r1 any, r2 ...any) null"
		check(t, code, "yak", ssaRange, wantLabel, "")
	})

	// t.Run("check method hover", func(t *testing.T) {
	// 	ssaParseRange := &ypb.Range{
	// 		Code:        "rsp.Data",
	// 		StartLine:   3,
	// 		StartColumn: 0,
	// 		EndLine:     3,
	// 		EndColumn:   8,
	// 	}
	// 	// 标准库函数
	// 	want := "```go\n" + `func Data() string` + "\n```"
	// 	check(t, code, "yak", ssaParseRange, want)
	// })
}
