package yakgrpc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func GetSuggestion(local ypb.YakClient, typ, pluginType string, t *testing.T, code string, Range *ypb.Range, id string) *ypb.YaklangLanguageSuggestionResponse {
	t.Log("========== get ", typ)
	ret, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   typ,
		YakScriptType: pluginType,
		YakScriptCode: code,
		Range:         Range,
		ModelID:       id,
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

	getCompletion := func(t *testing.T, code string, r *ypb.Range, ids ...string) *ypb.YaklangLanguageSuggestionResponse {
		var id string
		if len(ids) == 0 {
			id = uuid.NewString()
		} else {
			id = ids[0]
		}
		// if strings.HasSuffix(code, ".") {
		// 	tmpCode := strings.TrimSuffix(code, ".")
		// 	GetSuggestion(local, "completion", "yak", t, tmpCode, Range, id)
		// }
		return GetSuggestion(local, COMPLETION, "yak", t, code, r, id)
	}
	type callbackTyp func(suggestions []*ypb.SuggestionDescription)

	checkCompletionWithCallbacks := func(t *testing.T, code string, r *ypb.Range, callbacks ...callbackTyp) {
		t.Helper()
		var id string

		res := getCompletion(t, code, r, id)
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("should get completion but not")
		}
		for _, callback := range callbacks {
			callback(res.SuggestionMessage)
		}
	}

	labelsContainsCallback := func(t *testing.T, want []string) callbackTyp {
		return func(suggestions []*ypb.SuggestionDescription) {
			t.Helper()
			labels := lo.Map(suggestions, func(item *ypb.SuggestionDescription, _ int) string {
				return item.Label
			})
			if !utils.StringSliceContainsAll(labels, want...) {
				t.Fatalf("want %v, but got %v", want, labels)
			}
		}
	}

	labelsNotContainsCallback := func(t *testing.T, notWant []string) callbackTyp {
		return func(suggestions []*ypb.SuggestionDescription) {
			t.Helper()
			labels := lo.Map(suggestions, func(item *ypb.SuggestionDescription, _ int) string {
				return item.Label
			})
			if utils.ContainsAny(labels, notWant...) {
				t.Fatalf("don't want %v, but got", notWant)
			}
		}
	}

	checkCompletionContains := func(t *testing.T, code string, r *ypb.Range, want []string) {
		t.Helper()
		checkCompletionWithCallbacks(t, code, r, labelsContainsCallback(t, want))
	}

	t.Run("before symbols", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t, `a = 1; b = 2; c = 3;`, &ypb.Range{
			Code:        "",
			StartLine:   1,
			StartColumn: 21,
			EndLine:     1,
			EndColumn:   22,
		}, []string{"a", "b", "c"})
	})

	t.Run("assign variable offset", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t,
			`a = ssa.Parse("")~.`,
			&ypb.Range{
				Code:        ".",
				StartLine:   1,
				StartColumn: 19,
				EndLine:     1,
				EndColumn:   20,
			}, []string{"Ref", "Program"})
	})

	t.Run("before with repeated symbols", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`a = 1; a = () => 2;`,
			&ypb.Range{
				Code:        "",
				StartLine:   1,
				StartColumn: 19,
				EndLine:     1,
				EndColumn:   20,
			},
			func(suggestions []*ypb.SuggestionDescription) {
				// check only one "a"
				items := lo.Filter(suggestions, func(item *ypb.SuggestionDescription, _ int) bool {
					return item.Label == "a"
				})
				require.Len(t, items, 1, `want only 1 "a" label but got 2`)

				// check the "a" is a function
				item := items[0]
				require.Equal(t, "Function", item.Kind)
			})
	})

	t.Run("function returns", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t, `r = poc.Get("123")~; r.`, &ypb.Range{
			Code:        "r.",
			StartLine:   1,
			StartColumn: 22,
			EndLine:     1,
			EndColumn:   24,
		}, []string{"Length", "Pop"})
	})

	t.Run("type builtin methods", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t, `a = "asd"
a.`, &ypb.Range{
			Code:        "a.",
			StartLine:   2,
			StartColumn: 1,
			EndLine:     2,
			EndColumn:   3,
		}, []string{"Contains"})
	})

	t.Run("basic extern-lib completion", func(t *testing.T) {
		t.Parallel()

		res := getCompletion(t, `cli.`, &ypb.Range{
			Code:        "cli.",
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   5,
		})
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("code `cli.` should get completion but not")
		}
	})

	t.Run("extern struct completion", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t, `
prog = ssa.Parse("")~
prog.`, &ypb.Range{
			Code:        "prog.",
			StartLine:   3,
			StartColumn: 1,
			EndLine:     3,
			EndColumn:   7,
		}, []string{"Program", "Ref"})
	})

	t.Run("anonymous field struct completion", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t, `
rsp, err = http.Request("GET", "https://baidu.com")
rsp.`, &ypb.Range{
			Code:        "rsp.",
			StartLine:   3,
			StartColumn: 1,
			EndLine:     3,
			EndColumn:   5,
		}, []string{"Response", "Body", "Status", "Data"})
	})
	// 	t.Run("cache", func(t *testing.T) {
	// 		t.Parallel()
	// 		code := `asd = fuzz.HTTPRequest("")~
	// for a in asd.GetCommonParams() {
	// v = a
	// a
	// }`
	// 		id := uuid.NewString()
	// 		// // trigger cache
	// 		// getCompletion(t, code, &ypb.Range{
	// 		// 	Code:        "a",
	// 		// 	StartLine:   4,
	// 		// 	StartColumn: 0,
	// 		// 	EndLine:     4,
	// 		// 	EndColumn:   1,
	// 		// }, id)

	// 		// // check cache
	// 		code = strings.Replace(code, "\na\n", "\na.\n", 1)
	// 		checkCompletionContains(t, code, &ypb.Range{
	// 			Code:        "a.",
	// 			StartLine:   4,
	// 			StartColumn: 0,
	// 			EndLine:     4,
	// 			EndColumn:   2,
	// 		}, []string{"Fuzz", "Value", "Name"}, id)
	// 	})

	t.Run("chain function call", func(t *testing.T) {
		t.Parallel()
		checkCompletionContains(t, `
fuzz.HTTPRequest("")~.FuzzCookie("a","b").`, &ypb.Range{
			Code:        "",
			StartLine:   2,
			StartColumn: 42,
			EndLine:     2,
			EndColumn:   43,
		}, []string{"Exec"})
	})

	t.Run("alias extern-lib", func(t *testing.T) {
		t.Parallel()
		checkCompletionContains(t, `
a = cli
a.`, &ypb.Range{
			Code:        "a.",
			StartLine:   3,
			StartColumn: 1,
			EndLine:     3,
			EndColumn:   3,
		}, []string{"check"})
	})

	t.Run("trim code", func(t *testing.T) {
		t.Parallel()
		checkCompletionContains(t,
			`ssa.Parse("1", ssa.)`,
			&ypb.Range{
				Code:        "ssa.",
				StartLine:   1,
				StartColumn: 16,
				EndLine:     1,
				EndColumn:   20,
			}, []string{"Parse"})
	})

	t.Run("bytes builtin method", func(t *testing.T) {
		t.Parallel()
		checkCompletionContains(t,
			`rsp, _ = poc.HTTP("")~
rsp.`,
			&ypb.Range{
				Code:        "rsp.",
				StartLine:   2,
				StartColumn: 1,
				EndLine:     2,
				EndColumn:   5,
			}, []string{"Contains"})
	})

	t.Run("fix unexpected lib function completion", func(t *testing.T) {
		t.Parallel()
		checkCompletionWithCallbacks(t,
			`ssa`,
			&ypb.Range{
				Code:        "ssa",
				StartLine:   1,
				StartColumn: 1,
				EndLine:     1,
				EndColumn:   4,
			},
			labelsContainsCallback(t, []string{"println"}),
			labelsNotContainsCallback(t, []string{"Parse"}),
		)
	})

	t.Run("orType completion", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t,
			`a = x.If(true, [1, 2], {"a":1});a.`,
			&ypb.Range{
				Code:        ".",
				StartLine:   1,
				StartColumn: 34,
				EndLine:     1,
				EndColumn:   35,
			}, []string{"Pop", "Keys"})
	})
}

var local ypb.YakClient = nil

func CheckHover(t *testing.T) func(t *testing.T, code, scriptType string, Range *ypb.Range, want string, subStr ...bool) {
	if local == nil {
		var err error
		local, err = NewLocalClient()
		if err != nil {
			t.Fatal(err)
		}
	}

	getHover := func(t *testing.T, code, scriptType string, Range *ypb.Range, ids ...string) *ypb.YaklangLanguageSuggestionResponse {
		var id string
		if len(ids) == 0 {
			id = uuid.NewString()
		} else {
			id = ids[0]
		}
		return GetSuggestion(local, HOVER, scriptType, t, code, Range, id)
	}
	check := func(t *testing.T, code, scriptType string, Range *ypb.Range, want string, sub ...bool) {
		subStr := false
		for _, v := range sub {
			if v {
				subStr = true
				break
			}
		}

		req := getHover(t, code, scriptType, Range)
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

	getSignature := func(t *testing.T, code, typ string, Range *ypb.Range, ids ...string) *ypb.YaklangLanguageSuggestionResponse {
		var id string
		if len(ids) == 0 {
			id = uuid.NewString()
		} else {
			id = ids[0]
		}
		return GetSuggestion(local, SIGNATURE, typ, t, code, Range, id)
	}
	check := func(t *testing.T, code, typ string, Range *ypb.Range, wantLabel string, wantDesc string, sub ...bool) {
		subStr := false
		for _, v := range sub {
			if v {
				subStr = true
				break
			}
		}

		req := getSignature(t, code, typ, Range)
		log.Info(req.SuggestionMessage)
		require.Equal(t, 1, len(req.SuggestionMessage), "should get 1 suggestion")
		got := req.SuggestionMessage[0].Label
		if subStr {
			require.Contains(t, got, wantLabel)
		} else {
			require.Equal(t, wantLabel, got)
		}
		got = req.SuggestionMessage[0].Description
		if subStr {
			require.Contains(t, got, wantDesc)
		} else {
			require.Equal(t, wantDesc, got)
		}
	}
	return check
}

type CheckItem struct {
	name      string
	want      string
	Range     *ypb.Range
	subString bool
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_Basic(t *testing.T) {
	check := CheckHover(t)

	data := []CheckItem{
		{
			name: "a",
			want: "```go\ntype a number\n```",
			Range: &ypb.Range{
				Code:        "a",
				StartLine:   2,
				StartColumn: 1,
				EndLine:     2,
				EndColumn:   2,
			},
		},

		{
			name: "b",
			want: "```go\ntype b number\n```",
			Range: &ypb.Range{
				Code:        "b",
				StartLine:   3,
				StartColumn: 1,
				EndLine:     3,
				EndColumn:   2,
			},
		},
		{
			name: "c",
			want: "```go\ntype c string\n```",
			Range: &ypb.Range{
				Code:        "c",
				StartLine:   4,
				StartColumn: 1,
				EndLine:     4,
				EndColumn:   2,
			},
		},
		{
			name: "d",
			want: "```go\ntype d []byte\n```",
			Range: &ypb.Range{
				Code:        "d",
				StartLine:   5,
				StartColumn: 1,
				EndLine:     5,
				EndColumn:   2,
			},
		},
		{
			name: "d2",
			want: "```go\ntype d2 []byte\n```",
			Range: &ypb.Range{
				Code:        "d2",
				StartLine:   5,
				StartColumn: 13,
				EndLine:     5,
				EndColumn:   15,
			},
		},
		{
			name: "e",
			want: "```go\ntype e map[string]number\n```",
			Range: &ypb.Range{
				Code:        "e",
				StartLine:   6,
				StartColumn: 1,
				EndLine:     6,
				EndColumn:   2,
			},
		},
		{
			name: "f",
			want: "```go\ntype f []number\n```",
			Range: &ypb.Range{
				Code:        "f",
				StartLine:   7,
				StartColumn: 1,
				EndLine:     7,
				EndColumn:   2,
			},
		},
		{
			name: "g",
			want: "```go\ntype g chan number\n```",
			Range: &ypb.Range{
				Code:        "g",
				StartLine:   8,
				StartColumn: 1,
				EndLine:     8,
				EndColumn:   2,
			},
		},
		{
			name: "h",
			want: "```go\ntype h map[string]number\n```",
			Range: &ypb.Range{
				Code:        "h",
				StartLine:   9,
				StartColumn: 1,
				EndLine:     9,
				EndColumn:   2,
			},
		},
		{
			name: "i",
			want: "```go\ntype i number\n```",
			Range: &ypb.Range{
				Code:        "i",
				StartLine:   10,
				StartColumn: 1,
				EndLine:     10,
				EndColumn:   2,
			},
		},
		{
			name: "i",
			want: "```go\ntype i number\n```",
			Range: &ypb.Range{
				Code:        "i",
				StartLine:   10,
				StartColumn: 8,
				EndLine:     10,
				EndColumn:   9,
			},
		},
	}
	code := `
a = 1
b = 1.1
c = "asd"
d = b"asd"; d2 = []byte("asd")
e = {"a": 1}
f = [1, 2, 3]
g = make(chan int)
h = {"i":1}
i = h.i
`

	for _, item := range data {
		item := item
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			check(t, code, "yak", item.Range, item.want, item.subString)
		})
	}
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_Mitm(t *testing.T) {
	t.Run("check mitm hover argument", func(t *testing.T) {
		t.Parallel()

		check := CheckHover(t)
		check(t, `
		hijackSaveHTTPFlow = func(flow /* *schema.HTTPFlow */, modify /* func(modified *schema.HTTPFlow) */, drop/* func() */) {
			responseBytes, _ = codec.StrconvUnquote(flow.Response)
			a = flow.BeforeSave() //error
		}
		`,
			"mitm",
			&ypb.Range{
				Code:        "modify",
				StartLine:   2,
				StartColumn: 57,
				EndLine:     2,
				EndColumn:   64,
			},
			"```go\nfunc modify(r1 schema.HTTPFlow) null\n```",
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_ExternLib(t *testing.T) {
	check := CheckHover(t)
	codeTemplate := `%s {
prog  = ssa.Parse(
    "code", 
    ssa.withLanguage(
        ssa.Javascript
    )
)~
prog.Packages
}`

	data := []CheckItem{
		{
			name: "extern lib",
			want: getExternLibDesc("ssa"),
			Range: &ypb.Range{
				Code:        "ssa",
				StartLine:   2,
				StartColumn: 9,
				EndLine:     2,
				EndColumn:   12,
			},
		},
		{
			name: "extern lib method",
			want: getFuncDeclDesc(getFuncDeclByName("ssa", "Parse"), "Parse"),
			Range: &ypb.Range{
				Code:        "ssa.Parse",
				StartLine:   2,
				StartColumn: 9,
				EndLine:     2,
				EndColumn:   18,
			},
		},
		{
			name: "extern lib instance",
			want: getConstInstanceDesc(getInstanceByName("ssa", "Javascript")),
			Range: &ypb.Range{
				Code:        "ssa.Javascript",
				StartLine:   5,
				StartColumn: 9,
				EndLine:     5,
				EndColumn:   23,
			},
		},
		{
			name: "extern lib method return",
			want: `func (Program) Ref(name string) Value`,
			Range: &ypb.Range{
				Code:        "prog",
				StartLine:   2,
				StartColumn: 1,
				EndLine:     2,
				EndColumn:   5,
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
			item := item

			t.Run(fmt.Sprintf("test %s %s", item.name, testName), func(t *testing.T) {
				// t.Parallel()

				check(t, code, "yak", item.Range, item.want, item.subString)
			})
		}
	}
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_AliasExternLib(t *testing.T) {
	t.Run("alias lib", func(t *testing.T) {
		t.Parallel()
		check := CheckHover(t)

		check(t, `
a = ssa
a`,
			"yak",
			&ypb.Range{
				Code:        "a",
				StartLine:   3,
				StartColumn: 1,
				EndLine:     3,
				EndColumn:   2,
			},
			getExternLibDesc("ssa"),
		)
	})

	t.Run("alias lib instance", func(t *testing.T) {
		t.Parallel()
		check := CheckHover(t)

		check(t, `
a = ssa
a.Javascript`,
			"yak",
			&ypb.Range{
				Code:        "a.Javascript",
				StartLine:   3,
				StartColumn: 1,
				EndLine:     3,
				EndColumn:   13,
			},
			getConstInstanceDesc(getInstanceByName("ssa", "Javascript")),
		)
	})

	t.Run("alias lib function", func(t *testing.T) {
		t.Parallel()
		check := CheckHover(t)

		check(t, `
a = ssa
a.Parse()`,
			"yak",
			&ypb.Range{
				Code:        "a.Parse",
				StartLine:   3,
				StartColumn: 1,
				EndLine:     3,
				EndColumn:   8,
			},
			getFuncDeclDesc(getFuncDeclByName("ssa", "Parse"), "Parse"),
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_StructMemberAndMethod(t *testing.T) {
	check := CheckHover(t)
	code := `rsp, err = http.Request("GET", "https://baidu.com")
rsp.Status
rsp.Data()`
	t.Run("check member hover", func(t *testing.T) {
		t.Parallel()

		ssaRange := &ypb.Range{
			Code:        "rsp.Status",
			StartLine:   2,
			StartColumn: 1,
			EndLine:     2,
			EndColumn:   11,
		}
		want := "```go\n" + `field Status string` + "\n```"
		check(t, code, "yak", ssaRange, want)
	})

	t.Run("check method hover", func(t *testing.T) {
		t.Parallel()

		ssaParseRange := &ypb.Range{
			Code:        "rsp.Data",
			StartLine:   3,
			StartColumn: 1,
			EndLine:     3,
			EndColumn:   9,
		}
		// 标准库函数
		want := "```go\n" + `func (http_struct.YakHttpResponse) Data() string` + "\n```"
		check(t, code, "yak", ssaParseRange, want)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_FunctionReturns(t *testing.T) {
	t.Parallel()

	check := CheckHover(t)
	check(t,
		`r = poc.Get("123")~`,
		"yak",
		&ypb.Range{
			Code:        "r",
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   2,
		},
		"```go\ntype r [lowhttp.LowhttpResponse,http.Request]\n```",
	)
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_ForPhi(t *testing.T) {
	t.Parallel()

	check := CheckHover(t)
	check(t,
		`for user in ["user", "admin"] {
    for pass in ["pass", "123456"] {
        print(user, pass)
	}
}`,
		"yak",
		&ypb.Range{
			Code:        "user",
			StartLine:   3,
			StartColumn: 15,
			EndLine:     3,
			EndColumn:   19,
		},
		"```go\ntype user string\n```",
	)
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionSignature(t *testing.T) {
	check := CheckSignature(t)
	code := `a = func(b, c...) {}
a()
poc.HTTP()
c = poc.HTTP
c()
d = ""
d.Contains("c")
`

	pocLabel := "HTTP(i any, opts ...PocConfigOption) (rsp []byte, req []byte, err error)"
	pocDesc := "HTTP 发送请求并且返回原始响应报文，原始请求报文以及错误，它的第一个参数可以接收 []byte, string, http.Request 结构体，接下来可以接收零个到多个请求选项，用于对此次请求进行配置，例如设置超时时间，或者修改请求报文等\n\nExample:\n```\npoc.HTTP(\"GET / HTTP/1.1\\r\\nHost: www.yaklang.com\\r\\n\\r\\n\", poc.https(true), poc.replaceHeader(\"AAA\", \"BBB\")) // yaklang.com发送一个基于HTTPS协议的GET请求，并且添加一个请求头AAA，它的值为BBB\n```\n"

	t.Run("standard library function signature", func(t *testing.T) {
		t.Parallel()

		ssaRange := &ypb.Range{
			Code:        "poc.HTTP",
			StartLine:   3,
			StartColumn: 1,
			EndLine:     3,
			EndColumn:   9,
		}
		check(t, code, "yak", ssaRange, pocLabel, pocDesc)
	})
	t.Run("user function signature", func(t *testing.T) {
		t.Parallel()

		ssaRange := &ypb.Range{
			Code:        "a",
			StartLine:   2,
			StartColumn: 1,
			EndLine:     2,
			EndColumn:   2,
		}
		wantLabel := "func a(r1 any, r2 ...any) null"
		check(t, code, "yak", ssaRange, wantLabel, "")
	})

	t.Run("alias function signature", func(t *testing.T) {
		t.Parallel()

		ssaRange := &ypb.Range{
			Code:        "c",
			StartLine:   5,
			StartColumn: 1,
			EndLine:     5,
			EndColumn:   2,
		}
		check(t, code, "yak", ssaRange, pocLabel, pocDesc)
	})

	t.Run("type builtin method", func(t *testing.T) {
		t.Parallel()

		ssaRange := &ypb.Range{
			Code:        "d.Contains",
			StartLine:   7,
			StartColumn: 1,
			EndLine:     7,
			EndColumn:   11,
		}
		check(t, code, "yak", ssaRange, "func (string) Contains(r1 string) boolean", "判断字符串是否包含子串")
	})
}
