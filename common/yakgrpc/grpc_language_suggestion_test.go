package yakgrpc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
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

	getCompletion := func(t *testing.T, code string, r *ypb.Range, pluginType string, ids ...string) *ypb.YaklangLanguageSuggestionResponse {
		var id string
		if len(ids) == 0 {
			id = uuid.NewString()
		} else {
			id = ids[0]
		}
		return GetSuggestion(local, COMPLETION, pluginType, t, code, r, id)
	}
	type callbackTyp func(suggestions []*ypb.SuggestionDescription)

	checkMITMCompletionWithCallbacks := func(t *testing.T, code string, r *ypb.Range, callbacks ...callbackTyp) {
		t.Helper()
		var id string

		res := getCompletion(t, code, r, "mitm", id)
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("should get completion but not")
		}
		for _, callback := range callbacks {
			callback(res.SuggestionMessage)
		}
	}

	checkCompletionWithCallbacks := func(t *testing.T, code string, r *ypb.Range, callbacks ...callbackTyp) {
		t.Helper()
		var id string

		res := getCompletion(t, code, r, "yak", id)
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("should get completion but not")
		}
		for _, callback := range callbacks {
			callback(res.SuggestionMessage)
		}
	}

	checkCompletionWithIDCallbacks := func(t *testing.T, code string, r *ypb.Range, id string, callbacks ...callbackTyp) {
		t.Helper()
		res := getCompletion(t, code, r, "yak", id)
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

	getExactSuggestion := func(t *testing.T, suggestions []*ypb.SuggestionDescription, label string) *ypb.SuggestionDescription {
		items := lo.Filter(suggestions, func(item *ypb.SuggestionDescription, _ int) bool {
			return item.Label == label
		})
		require.Lenf(t, items, 1, `want only 1 %s but not`, label)
		return items[0]
	}

	t.Run("object", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t, `NewThreadPool = func(size){
threadPool = {
	"consumer":f =>{
		return threadPool
	},
	"aaa": 1
}
return threadPool
}
pool = NewThreadPool(10)
pool.`, &ypb.Range{
			Code:        "pool.",
			StartLine:   11,
			StartColumn: 1,
			EndLine:     11,
			EndColumn:   6,
		}, func(suggestions []*ypb.SuggestionDescription) {
			item := getExactSuggestion(t, suggestions, "consumer")
			require.Equal(t, "Method", item.Kind)
			require.Equal(t, "consumer(${1:any})", item.InsertText)
			item = getExactSuggestion(t, suggestions, "aaa")
			require.Equal(t, "Field", item.Kind)
			require.Equal(t, "aaa", item.InsertText)
			require.Equal(t, "number", item.Description)
		})
	})

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
				item := getExactSuggestion(t, suggestions, "a")
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
		}, "yak")
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

	t.Run("extern alias type completion", func(t *testing.T) {
		t.Parallel()

		checkCompletionContains(t, `
dur = time.ParseDuration("100ms")~
dur.`, &ypb.Range{
			Code:        "dur.",
			StartLine:   3,
			StartColumn: 1,
			EndLine:     3,
			EndColumn:   5,
		}, []string{"Abs", "Hours", "Minutes"})
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

	t.Run("fix map field and func", func(t *testing.T) {
		t.Parallel()

		t.Run("map field", func(t *testing.T) {
			checkCompletionWithCallbacks(t,
				`a = {b"field": 1};a.`,
				&ypb.Range{
					Code:        "a.",
					StartLine:   1,
					StartColumn: 20,
					EndLine:     1,
					EndColumn:   21,
				},
				labelsContainsCallback(t, []string{"field", "Keys"}),
				func(suggestions []*ypb.SuggestionDescription) {
					item := getExactSuggestion(t, suggestions, "field")
					require.Equal(t, "field", item.InsertText)
				},
			)
		})

		t.Run("map function", func(t *testing.T) {
			checkCompletionWithCallbacks(t,
				`a = {"func": func(b, c) {return 2}};a.`,
				&ypb.Range{
					Code:        "a.",
					StartLine:   1,
					StartColumn: 37,
					EndLine:     1,
					EndColumn:   39,
				},
				func(suggestions []*ypb.SuggestionDescription) {
					// check
					item := getExactSuggestion(t, suggestions, "func")
					require.Equal(t, "Method", item.Kind)
					require.Equal(t, getFuncCompletionBySSAType("func",
						ssa.NewFunctionTypeDefine("func", []ssa.Type{ssa.CreateAnyType(), ssa.CreateAnyType()}, []ssa.Type{ssa.CreateNumberType()}, false)),
						item.InsertText)
				},
			)
		})
	})

	t.Run("inner struct", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`rsp, req = poc.Get("")~;flow=rsp.RedirectRawPackets[0];flow.`,
			&ypb.Range{
				Code:        "flow.",
				StartLine:   1,
				StartColumn: 57,
				EndLine:     1,
				EndColumn:   62,
			},
			labelsContainsCallback(t, []string{"Request", "Response", "IsHttps", "RespRecord"}),
		)
	})

	t.Run("halfway-string", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`a = "";a.has`,
			&ypb.Range{
				Code:        "a.has",
				StartLine:   1,
				StartColumn: 8,
				EndLine:     1,
				EndColumn:   13,
			},
			labelsContainsCallback(t, []string{"HasPrefix"}),
			labelsNotContainsCallback(t, []string{"has", "poc"}),
		)
	})

	t.Run("halfway-map", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`a = {"a":1};a.del`,
			&ypb.Range{
				Code:        "a.del",
				StartLine:   1,
				StartColumn: 13,
				EndLine:     1,
				EndColumn:   18,
			},
			labelsContainsCallback(t, []string{"Delete"}),
			labelsNotContainsCallback(t, []string{"del", "poc"}),
		)
	})

	t.Run("no completion with slice", func(t *testing.T) {
		t.Parallel()
		code := `a = []int{1};member=a[4];member.`
		r := &ypb.Range{
			Code:        `member.`,
			StartLine:   1,
			StartColumn: 26,
			EndLine:     1,
			EndColumn:   33,
		}
		res := getCompletion(t, code, r, "yak", "")
		labelsNotContainsCallback(t, []string{"Append"})(res.SuggestionMessage)
	})

	t.Run("no completion in member  with map", func(t *testing.T) {
		t.Parallel()
		code := `a = map[string]int{};member=a.b;member.`
		r := &ypb.Range{
			Code:        `member.`,
			StartLine:   1,
			StartColumn: 33,
			EndLine:     1,
			EndColumn:   40,
		}
		res := getCompletion(t, code, r, "yak", "")
		labelsNotContainsCallback(t, []string{"Append"})(res.SuggestionMessage)
	})

	t.Run("halfway-slice", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`a = [];a.app`,
			&ypb.Range{
				Code:        "a.app",
				StartLine:   1,
				StartColumn: 8,
				EndLine:     1,
				EndColumn:   13,
			},
			labelsContainsCallback(t, []string{"Append"}),
			labelsNotContainsCallback(t, []string{"app", "poc"}),
		)
	})

	t.Run("halfway-lib", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`freq, err = fuzz.htt`,
			&ypb.Range{
				Code:        "fuz.htt",
				StartLine:   1,
				StartColumn: 13,
				EndLine:     1,
				EndColumn:   21,
			},
			labelsContainsCallback(t, []string{"https", "HTTPRequest", "FuzzCalcExpr"}),
		)
	})
	t.Run("halfway-struct", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`rsp, req = poc.HTTPEx("")~
rsp.Ba`,
			&ypb.Range{
				Code:        "rsp.Ba",
				StartLine:   2,
				StartColumn: 1,
				EndLine:     2,
				EndColumn:   7,
			},
			labelsContainsCallback(t, []string{"Https", "BareResponse", "Proxy", "RawPacket"}),
			labelsNotContainsCallback(t, []string{"Len", "Push", "Pop"}), // should not be string method
		)
	})

	t.Run("completion with multi-bytes chars before", func(t *testing.T) {
		t.Parallel()

		res := getCompletion(t, "//前面是一些注释，用于测试\ncli.\n//后面也是一些注释，用于测试", &ypb.Range{
			Code:        "cli.",
			StartLine:   2,
			StartColumn: 1,
			EndLine:     2,
			EndColumn:   5,
		}, "yak")
		if len(res.SuggestionMessage) == 0 {
			t.Fatal("code `cli.` should get completion but not")
		}
	})

	t.Run("defer expression", func(t *testing.T) {
		t.Parallel()

		id, token := utils.RandStringBytes(16), utils.RandStringBytes(16)
		checkCompletionWithIDCallbacks(t, fmt.Sprintf(`m = {"%s":"d"}
defer m.`, token), &ypb.Range{
			Code: "m.", StartLine: 2, StartColumn: 7, EndLine: 8, EndColumn: 9,
		}, id,
			labelsContainsCallback(t, []string{"Delete", "Keys", token}),
		)
	})

	t.Run("mitm pluginType completion", func(t *testing.T) {
		t.Parallel()
		code := `hijackSaveHTTPFlow = func(flow, modify, drop) {
println(PLUGIN_RUNTIME_ID)

}
		`
		checkMITMCompletionWithCallbacks(t, code, &ypb.Range{
			Code:        "",
			StartLine:   3,
			StartColumn: 1,
			EndLine:     3,
			EndColumn:   1,
		},
			labelsContainsCallback(t, []string{"print", consts.PLUGIN_CONTEXT_KEY_RUNTIME_ID, "MITM_PARAMS"}),
			func(suggestions []*ypb.SuggestionDescription) {
				item := getExactSuggestion(t, suggestions, consts.PLUGIN_CONTEXT_KEY_RUNTIME_ID)
				require.Equal(t, "Variable", item.Kind)
				require.Equal(t, "string", item.Description)
				item = getExactSuggestion(t, suggestions, "MITM_PARAMS")
				require.Equal(t, "Variable", item.Kind)
				require.Equal(t, "map[string]string", item.Description)
			},
		)
	})

	t.Run("fix-function-params", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`dyn.IsYakFunc(app)`,
			&ypb.Range{
				Code:        "app",
				StartLine:   1,
				StartColumn: 15,
				EndLine:     1,
				EndColumn:   18,
			},
			func(suggestions []*ypb.SuggestionDescription) {
				item := getExactSuggestion(t, suggestions, "append")
				require.Equal(t, "Function", item.Kind)
				require.Equal(t, "append", item.InsertText)
			},
		)
	})

	t.Run("fix-before-paren", func(t *testing.T) {
		t.Parallel()

		checkCompletionWithCallbacks(t,
			`dyn.IsYakFunc(app())`,
			&ypb.Range{
				Code:        "app",
				StartLine:   1,
				StartColumn: 15,
				EndLine:     1,
				EndColumn:   18,
			},
			func(suggestions []*ypb.SuggestionDescription) {
				item := getExactSuggestion(t, suggestions, "append")
				require.Equal(t, "Function", item.Kind)
				require.Equal(t, "append(${1:a}, ${2:vals...})", item.InsertText)
			},
		)
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
			want: _markdownWrapper("type a number"),
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
			want: _markdownWrapper("type b number"),
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
			want: _markdownWrapper("type c string"),
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
			want: _markdownWrapper("type d []byte"),
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
			want: _markdownWrapper("type d2 []byte"),
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
			want: _markdownWrapper("type e map[string]number"),
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
			want: _markdownWrapper("type f []number"),
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
			want: _markdownWrapper("type g chan number"),
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
			want: _markdownWrapper("type h map[string]number"),
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
			want: _markdownWrapper("type i number"),
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
			want: _markdownWrapper("type i number"),
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
			"```go\nfunc modify(i1 schema.HTTPFlow) null\n```",
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_Generic(t *testing.T) {
	t.Run("x.Find", func(t *testing.T) {
		t.Parallel()

		check := CheckHover(t)
		check(t, `x.Find()`,
			"yak",
			&ypb.Range{
				Code:        "x.Find",
				StartLine:   1,
				StartColumn: 1,
				EndLine:     1,
				EndColumn:   8,
			},
			_markdownWrapper(`Find(i []T|map[U]T, fc (T) -> boolean) T`),
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
			want: getFuncDeclDesc(nil, getFuncDeclByName("ssa", "Parse")),
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
			getFuncDeclDesc(nil, getFuncDeclByName("ssa", "Parse")),
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_UnFinish_InputError(t *testing.T) {
	check := CheckHover(t)

	t.Run("library", func(t *testing.T) {
		t.Parallel()

		check(t, `
t = ""
host, port, _ = str.ParseStringToHostPort(t)
ssa.
`,
			"mitm",
			&ypb.Range{
				Code:        "ssa",
				StartLine:   4,
				StartColumn: 1,
				EndLine:     4,
				EndColumn:   4,
			},
			getExternLibDesc("ssa"),
		)
	})

	t.Run("defer", func(t *testing.T) {
		t.Parallel()

		check(t, `
a = 1
m = {"asd": 1}
defer m.
`,
			"mitm",
			&ypb.Range{
				Code:        "m.",
				StartLine:   4,
				StartColumn: 7,
				EndLine:     4,
				EndColumn:   9,
			},
			_markdownWrapper("type m map[string]number"),
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

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_Function(t *testing.T) {
	t.Parallel()

	t.Run("function", func(t *testing.T) {
		check := CheckHover(t)
		check(t,
			`r = poc.Get("123")~`,
			"yak",
			&ypb.Range{
				Code:        "poc.Get",
				StartLine:   1,
				StartColumn: 5,
				EndLine:     1,
				EndColumn:   12,
			},
			`Get 向指定 URL 发送 GET 请求并且返回响应结构体`,
			true,
		)
	})

	t.Run("function return", func(t *testing.T) {
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
	})
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

func TestGRPCMUSTPASS_LANGUAGE_SuggestionHover_FixFunctionParams(t *testing.T) {
	t.Parallel()

	check := CheckHover(t)
	code := `dyn.IsYakFunc(x.Find)`

	ssaParseRange := &ypb.Range{
		Code:        "x.Find",
		StartLine:   1,
		StartColumn: 15,
		EndLine:     1,
		EndColumn:   21,
	}
	// 标准库函数
	check(t, code, "yak", ssaParseRange, _markdownWrapper(`Find(i []T|map[U]T, fc (T) -> boolean) T`))
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
e={"a":1}
e.Delete
`

	funcDecl := getFuncDeclByName("poc", "HTTP")
	pocLabel := getFuncDeclLabel(nil, funcDecl)
	pocDesc := funcDecl.Document

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
		wantLabel := "func a(i1 any, i2 ...any) null"
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

		t.Run("slice", func(t *testing.T) {
			ssaRange := &ypb.Range{
				Code:        "d.Contains",
				StartLine:   7,
				StartColumn: 1,
				EndLine:     7,
				EndColumn:   11,
			}
			check(t, code, "yak", ssaRange, "func (string) Contains(i1 string) boolean", "判断字符串是否包含子串")
		})

		t.Run("map", func(t *testing.T) {
			ssaRange := &ypb.Range{
				Code:        "e.Delete",
				StartLine:   9,
				StartColumn: 1,
				EndLine:     9,
				EndColumn:   9,
			}
			check(t, code, "yak", ssaRange, "func (map[string]number) Delete(i1 string) null", "移除一个值")
		})
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionSignature_Generic(t *testing.T) {
	t.Run("x.Find", func(t *testing.T) {
		t.Parallel()

		check := CheckSignature(t)
		check(t, `x.Find()`,
			"yak",
			&ypb.Range{
				Code:        "x.Find",
				StartLine:   1,
				StartColumn: 1,
				EndLine:     1,
				EndColumn:   8,
			},
			"Find(i []T|map[U]T, fc (T) -> boolean) T",
			"",
		)
	})
}

func TestGRPCMUSTPASS_LANGUAGE_SuggestionSignature_Generic_After_FunctionParams(t *testing.T) {
	t.Run("x.Find", func(t *testing.T) {
		t.Parallel()

		check := CheckSignature(t)
		check(t, `dyn.IsYakFunc(x.Find)
x.Find()`,
			"yak",
			&ypb.Range{
				Code:        "x.Find",
				StartLine:   2,
				StartColumn: 1,
				EndLine:     2,
				EndColumn:   8,
			},
			"Find(i []T|map[U]T, fc (T) -> boolean) T",
			"",
		)
	})
}

func Test_SyntaxflowCompletion(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("native-call", func(t *testing.T) {
		id := uuid.NewString()
		resp := GetSuggestion(local, "completion", "syntaxflow", t, `
<
		`, &ypb.Range{
			Code:        "<",
			StartLine:   2,
			StartColumn: 2,
			EndLine:     2,
			EndColumn:   3,
		}, id)
		require.True(t, len(resp.SuggestionMessage) > 0)
	})

	t.Run("library", func(t *testing.T) {
		id := uuid.NewString()
		resp := GetSuggestion(local, "completion", "syntaxflow", t, `
<include()>
`, &ypb.Range{
			Code:        "<include(",
			StartLine:   2,
			StartColumn: 10,
			EndLine:     2,
			EndColumn:   10,
		}, id)

		require.True(t, len(resp.SuggestionMessage) > 0)
	})
}

func Test_FuzztagCompletion(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("fuzztag name", func(t *testing.T) {
		id := uuid.NewString()
		resp := GetSuggestion(local, "completion", "fuzztag", t, `
{{
		`, &ypb.Range{
			Code:        "{{",
			StartLine:   2,
			StartColumn: 2,
			EndLine:     2,
			EndColumn:   3,
		}, id)
		require.True(t, len(resp.SuggestionMessage) > 0)
	})

	token := utils.RandStringBytes(10)
	err = yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase(), 0, &schema.YakScript{
		Type:       "codec",
		ScriptName: token,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), token)
	})

	t.Run("code plugin", func(t *testing.T) {
		id := uuid.NewString()
		resp := GetSuggestion(local, "completion", "fuzztag", t, fmt.Sprintf(`
{{codec(%s
`, token[:5]), &ypb.Range{
			Code:        fmt.Sprintf(`{{codec(%s`, token[:5]),
			StartLine:   2,
			StartColumn: 10,
			EndLine:     2,
			EndColumn:   10,
		}, id)
		require.True(t, len(resp.SuggestionMessage) > 0)

	})
}

func Test_FuzztagHover(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("fuzztag name", func(t *testing.T) {
		id := uuid.NewString()
		resp := GetSuggestion(local, "hover", "fuzztag", t, ``, &ypb.Range{
			Code:        "null",
			StartLine:   2,
			StartColumn: 2,
			EndLine:     2,
			EndColumn:   3,
		}, id)
		require.True(t, len(resp.SuggestionMessage) > 0)
		require.Contains(t, resp.SuggestionMessage[0].Label, "生成一个空字节，如果指定了数量，将生成指定数量的空字节 {{null(5)}} 表示生成 5 个空字节")
	})

}

func Test_NewFuzztagCompletion(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("fuzztag name", func(t *testing.T) {
		resp, err := client.FuzzTagSuggestion(context.Background(), &ypb.FuzzTagSuggestionRequest{
			HotPatchCode: "",
			FuzztagCode:  "{{",
			InspectType:  COMPLETION,
		})
		require.NoError(t, err)
		require.True(t, len(resp.SuggestionMessage) > 0)
	})

	t.Run("hotPatch", func(t *testing.T) {
		resp, err := client.FuzzTagSuggestion(context.Background(), &ypb.FuzzTagSuggestionRequest{
			HotPatchCode: "handle = func(){}",
			FuzztagCode:  "{{yak(",
			InspectType:  COMPLETION,
		})
		require.NoError(t, err)
		require.True(t, len(resp.SuggestionMessage) > 0)
		require.Contains(t, resp.SuggestionMessage[0].Label, "handle")
	})

	token := utils.RandStringBytes(10)
	err = yakit.CreateOrUpdateYakScript(consts.GetGormProfileDatabase(), 0, &schema.YakScript{
		Type:       "codec",
		ScriptName: token,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), token)
	})

	t.Run("code plugin", func(t *testing.T) {
		resp, err := client.FuzzTagSuggestion(context.Background(), &ypb.FuzzTagSuggestionRequest{
			FuzztagCode: fmt.Sprintf(`{{codec(%s`, token[:5]),
			InspectType: COMPLETION,
		})
		require.NoError(t, err)
		require.True(t, len(resp.SuggestionMessage) > 0)
	})

	groupName := utils.RandStringBytes(10)
	err = yakit.CreatePayload(consts.GetGormProfileDatabase(), "qqqq", groupName, "", 0, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		yakit.DeletePayloadByGroup(consts.GetGormProfileDatabase(), token)
	})

	t.Run("payload", func(t *testing.T) {
		resp, err := client.FuzzTagSuggestion(context.Background(), &ypb.FuzzTagSuggestionRequest{
			FuzztagCode: `{{payload(`,
			InspectType: COMPLETION,
		})
		require.NoError(t, err)
		require.True(t, len(resp.SuggestionMessage) > 0)
		utils.StringArrayContains(lo.Map(resp.SuggestionMessage, func(item *ypb.SuggestionDescription, _ int) string {
			return item.InsertText
		}), groupName)
	})

}

func Test_NewFuzztagHover(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("fuzztag name", func(t *testing.T) {

		resp, err := client.FuzzTagSuggestion(context.Background(), &ypb.FuzzTagSuggestionRequest{
			FuzztagCode: "null",
			InspectType: HOVER,
		})
		require.NoError(t, err)
		require.True(t, len(resp.SuggestionMessage) > 0)
		require.Contains(t, resp.SuggestionMessage[0].Label, "生成一个空字节，如果指定了数量，将生成指定数量的空字节 {{null(5)}} 表示生成 5 个空字节")
	})

}
